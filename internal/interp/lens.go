// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

type lensTag int

const (
	lDel lensTag = iota
	lStore
	lKey
	lLabel
	lValue
	lSeq
	lCounter
	lConcat
	lUnion
	lSubtree
	lStar
	lMaybe
	lSquare
	lRec
)

// Encoding markers for the tree side (mirrors ENC_EQ / ENC_SLASH in lens.h).
const (
	encEq    = "\x03"
	encSlash = "\x04"
)

// Lens is a compiled lens, mirroring the fields of struct lens that the get and
// put algorithms need. ctype is the concrete-text regexp whose capture-group
// structure drives register indexing during get; atype/ktype/vtype are the
// tree-side (put) regexps.
type Lens struct {
	tag   lensTag
	ctype *Regexp
	atype *Regexp // over the encoded tree; drives put splits
	ktype *Regexp // key regexp contributed to the enclosing subtree
	vtype *Regexp // value regexp

	// leaf data
	regexp *Regexp // del/store/key
	str    string  // label/value/seq/counter names, and del default text

	// structure
	children []*Lens // concat/union/square
	child    *Lens   // subtree/star/maybe

	// flags
	key           bool
	value         bool
	consumesValue bool
	recursive     bool

	// fullCapture forces every child of this concat to receive a capturing
	// wrapper group in the ctype, regardless of ctypeCaptures. It is set on a
	// square's inner 3-way concat, whose get/put walk (getSquare/parseSquare)
	// indexes each component's register directly.
	fullCapture bool

	// recursive placeholder resolution
	body     *Lens // resolved body for lRec
	resolved bool
}

func digitsRegexp() *Regexp { return newRegexp("[0-9]+", false) }

// emptyCtype is the ctype of a lens whose concrete text is empty and never read
// by the register walk (label/value/seq/counter). Unlike regexpMakeEmpty's "()"
// it contributes no capture group, so it costs nothing in the register array.
func emptyCtype() *Regexp { return newRegexp("", false) }

// ctypeCaptures reports whether l, when embedded as a concat operand, needs a
// capturing wrapper group in the parent's ctype — i.e. whether the get/put
// register walk reads l's own wrapper. Three cases read it:
//   - leaves that consume concrete text (del/store/key) read the matched text;
//   - star/square read the wrapper's start/end to learn their text span before
//     re-matching their child/inner-concat over it;
//   - a subtree transparently forwards to whichever of the above it wraps.
//
// The remaining structural nodes (concat/union/maybe) have their wrapper walked
// past without being read, so it can be non-capturing. Union branches are the
// exception (always captured, so getUnion can tell which alternative matched)
// and are handled by makeUnion directly.
func ctypeCaptures(l *Lens) bool {
	switch l.tag {
	case lDel, lStore, lKey, lStar, lSquare:
		return true
	case lSubtree:
		return ctypeCaptures(l.child)
	default:
		return false
	}
}

// concatCaptureFlags returns, for each child of a concat, whether its ctype
// wrapper is capturing. A fullCapture concat captures every child.
func concatCaptureFlags(children []*Lens, full bool) []bool {
	flags := make([]bool, len(children))
	for i, c := range children {
		flags[i] = full || ctypeCaptures(c)
	}
	return flags
}

// restrict removes the reserved encoding bytes from a key/value type so it
// cannot span the encoded-tree separators during put. Raw (automata-produced)
// types must go through the automata restriction; for other types a value that
// can never contain a reserved byte is returned unchanged as a fast path.
func restrict(r *Regexp) *Regexp {
	if r.cachedRestrict != nil {
		return r.cachedRestrict
	}
	res := newRawRegexp(restrictRE2(r.re2()))
	r.cachedRestrict = res
	return res
}

func makePrim(tag lensTag, re *Regexp, str string) *Lens {
	l := &Lens{tag: tag, regexp: re, str: str}
	l.key = tag == lKey || tag == lLabel || tag == lSeq
	l.value = tag == lStore || tag == lValue
	l.consumesValue = tag == lStore || tag == lValue
	l.atype = regexpMakeEmpty()
	switch tag {
	case lDel, lStore, lKey:
		l.ctype = re
	default: // label/value/seq/counter
		l.ctype = emptyCtype()
	}
	switch tag {
	case lKey:
		l.ktype = re
	case lLabel:
		l.ktype = makeRegexpLiteral(str)
	case lSeq:
		l.ktype = digitsRegexp()
	case lStore:
		l.vtype = re
	case lValue:
		l.vtype = makeRegexpLiteral(str)
	}
	return l
}

// firstType returns the first non-nil regexp (used to combine ktype/vtype of a
// concat, where at most one side carries a key or value).
func firstType(a, b *Regexp) *Regexp {
	if a != nil {
		return a
	}
	return b
}

func unionType(a, b *Regexp) *Regexp {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return regexpUnion(a, b)
	}
}

// flatOperands returns l's children when l is an n-ary node of tag, and [l]
// otherwise. Concatenation and union are associative, so splicing a same-tag
// operand's children into the parent yields a semantically identical lens with
// one fewer capture group per level. That matters because the ctype regexps are
// matched by Go's RE2 engine, whose thread-list step copies the whole capture
// array on every transition — cost that grows with the group count. Flattening
// the deeply left-nested trees the parser builds (`a . b . c . …` and
// `e1 | e2 | …`) roughly halves the groups on large lenses (e.g. Sshd) and with
// them the RE2 match cost, while the get/put walkers already iterate children
// n-ary via the same nreg accounting.
func flatOperands(l *Lens, tag lensTag) []*Lens {
	if l.tag == tag {
		return l.children
	}
	return []*Lens{l}
}

// mergeChildren concatenates the (possibly spliced) operand lists into a fresh
// slice, never aliasing an operand's backing array.
func mergeChildren(a, b []*Lens) []*Lens {
	out := make([]*Lens, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

func childCtypes(children []*Lens) []*Regexp {
	rs := make([]*Regexp, len(children))
	for i, c := range children {
		rs[i] = c.ctype
	}
	return rs
}

func childAtypes(children []*Lens) []*Regexp {
	rs := make([]*Regexp, len(children))
	for i, c := range children {
		rs[i] = c.atype
	}
	return rs
}

func makeConcat(l1, l2 *Lens) *Lens {
	children := mergeChildren(flatOperands(l1, lConcat), flatOperands(l2, lConcat))
	l := &Lens{tag: lConcat, children: children}
	l.ctype = regexpConcatCap(childCtypes(children), concatCaptureFlags(children, false))
	l.atype = regexpConcatN(childAtypes(children))
	l.ktype = firstType(l1.ktype, l2.ktype)
	l.vtype = firstType(l1.vtype, l2.vtype)
	l.consumesValue = l1.consumesValue || l2.consumesValue
	l.key = l1.key || l2.key
	l.value = l1.value || l2.value
	l.recursive = l1.recursive || l2.recursive
	return l
}

func makeUnion(l1, l2 *Lens) *Lens {
	children := mergeChildren(flatOperands(l1, lUnion), flatOperands(l2, lUnion))
	l := &Lens{tag: lUnion, children: children}
	l.ctype = regexpUnionN(childCtypes(children))
	l.atype = regexpUnionN(childAtypes(children))
	l.ktype = unionType(l1.ktype, l2.ktype)
	l.vtype = unionType(l1.vtype, l2.vtype)
	l.consumesValue = l1.consumesValue && l2.consumesValue
	l.key = l1.key || l2.key
	l.value = l1.value || l2.value
	l.recursive = l1.recursive || l2.recursive
	return l
}

// subtreeAtype encodes a subtree's key/value types into the tree alphabet:
// (kpat) ENC_EQ (vpat) ENC_SLASH.
func subtreeAtype(ktype, vtype *Regexp) *Regexp {
	kre := ""
	if ktype != nil {
		kre = restrict(ktype).re2()
	}
	vre := ""
	if vtype != nil {
		vre = restrict(vtype).re2()
	}
	return newRawRegexp("(" + kre + ")" + encEq + "(" + vre + ")" + encSlash)
}

func makeSubtree(c *Lens) *Lens {
	l := &Lens{tag: lSubtree, child: c}
	l.ctype = c.ctype
	l.atype = subtreeAtype(c.ktype, c.vtype)
	l.recursive = c.recursive
	return l
}

func makeStar(c *Lens) *Lens {
	l := &Lens{tag: lStar, child: c}
	l.ctype = regexpIter(c.ctype, 0, -1)
	l.atype = regexpIter(c.atype, 0, -1)
	l.recursive = c.recursive
	return l
}

func makePlus(c *Lens) *Lens {
	return makeConcat(c, makeStar(c))
}

func makeMaybe(c *Lens) *Lens {
	l := &Lens{tag: lMaybe, child: c}
	l.ctype = regexpMaybe(c.ctype)
	l.atype = regexpMaybe(c.atype)
	// The key/value contributed by an optional lens are themselves optional, so
	// a subtree whose value comes from a `?`-guarded store correctly admits an
	// empty (absent) value. This mirrors lns_make_maybe applying regexp_maybe to
	// every type, including ktype and vtype.
	l.ktype = regexpMaybe(c.ktype)
	l.vtype = regexpMaybe(c.vtype)
	l.key = c.key
	l.value = c.value
	l.recursive = c.recursive
	return l
}

// makeSquare builds square l1 l2 l3, which matches l1 . l2 . l3 where the text
// matched by l1 and l3 must be identical (a balanced delimiter). Its child is a
// flat 3-way concat, matching how upstream get_square walks the registers.
func makeSquare(l1, l2, l3 *Lens) *Lens {
	inner := &Lens{tag: lConcat, children: []*Lens{l1, l2, l3}, fullCapture: true}
	inner.ctype = regexpConcatN([]*Regexp{l1.ctype, l2.ctype, l3.ctype})
	inner.atype = regexpConcatN([]*Regexp{l1.atype, l2.atype, l3.atype})
	inner.ktype = firstType(firstType(l1.ktype, l2.ktype), l3.ktype)
	inner.vtype = firstType(firstType(l1.vtype, l2.vtype), l3.vtype)
	inner.consumesValue = l1.consumesValue || l2.consumesValue || l3.consumesValue
	inner.key = l1.key || l2.key || l3.key
	inner.value = l1.value || l2.value || l3.value
	inner.recursive = l1.recursive || l2.recursive || l3.recursive
	l := &Lens{tag: lSquare, child: inner}
	// Prefer the exact balanced-delimiter language (square_precise_type); fall
	// back to the loose l1 . body . l3 concatenation when the delimiter language
	// is infinite or too large. The child concat keeps the loose ctype, which
	// get_square re-matches to locate the delimiter/body boundaries.
	if precise := squarePreciseType(l1.ctype, l2.ctype); precise != nil {
		l.ctype = precise
	} else {
		l.ctype = inner.ctype
	}
	l.atype = inner.atype
	l.ktype = inner.ktype
	l.vtype = inner.vtype
	l.key = inner.key
	l.value = inner.value
	l.consumesValue = inner.consumesValue
	l.recursive = inner.recursive
	return l
}

// recomputeAtype rebuilds the tree-side types of a recursive lens after the
// recursion knot is tied. A recursive lens's own atype is finite (recursion is
// hidden inside subtree boundaries), so a single post-order pass — treating the
// recursive placeholder as a leaf with its already-tied types — fixes the
// concats that embed the placeholder (e.g. a block body `key . "{" . lns . "}"`
// whose atype was built while the placeholder's atype was still empty).
func recomputeAtype(l *Lens, seen map[*Lens]bool) {
	if l == nil || seen[l] || l.tag == lRec {
		return
	}
	seen[l] = true
	for _, c := range l.children {
		recomputeAtype(c, seen)
	}
	recomputeAtype(l.child, seen)

	switch l.tag {
	case lConcat:
		l.atype = regexpConcatN(atypesOf(l.children))
		l.ktype, l.vtype = nil, nil
		for _, c := range l.children {
			l.ktype = firstType(l.ktype, c.ktype)
			l.vtype = firstType(l.vtype, c.vtype)
		}
	case lUnion:
		l.atype = regexpUnionN(atypesOf(l.children))
		l.ktype, l.vtype = nil, nil
		for _, c := range l.children {
			l.ktype = unionType(l.ktype, c.ktype)
			l.vtype = unionType(l.vtype, c.vtype)
		}
	case lSubtree:
		l.atype = subtreeAtype(l.child.ktype, l.child.vtype)
	case lStar:
		l.atype = regexpIter(l.child.atype, 0, -1)
	case lMaybe:
		l.atype = regexpMaybe(l.child.atype)
		l.ktype = regexpMaybe(l.child.ktype)
		l.vtype = regexpMaybe(l.child.vtype)
	case lSquare:
		l.atype = l.child.atype
		l.ktype = l.child.ktype
		l.vtype = l.child.vtype
	}
}

func atypesOf(ls []*Lens) []*Regexp {
	out := make([]*Regexp, len(ls))
	for i, c := range ls {
		out[i] = c.atype
	}
	return out
}

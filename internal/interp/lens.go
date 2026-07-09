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

// Lens is a compiled lens, mirroring the fields of struct lens that the get
// algorithm needs. ctype is the concrete-text regexp whose capture-group
// structure drives register indexing during get.
type Lens struct {
	tag   lensTag
	ctype *Regexp

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

	// recursive placeholder resolution
	body     *Lens // resolved body for lRec
	resolved bool
}

func makePrim(tag lensTag, re *Regexp, str string) *Lens {
	l := &Lens{tag: tag, regexp: re, str: str}
	l.key = tag == lKey || tag == lLabel || tag == lSeq
	l.value = tag == lStore || tag == lValue
	l.consumesValue = tag == lStore || tag == lValue
	switch tag {
	case lDel, lStore, lKey:
		l.ctype = re
	default: // label/value/seq/counter
		l.ctype = regexpMakeEmpty()
	}
	return l
}

func makeConcat(l1, l2 *Lens) *Lens {
	l := &Lens{tag: lConcat, children: []*Lens{l1, l2}}
	l.ctype = regexpConcatN([]*Regexp{l1.ctype, l2.ctype})
	l.consumesValue = l1.consumesValue || l2.consumesValue
	l.recursive = l1.recursive || l2.recursive
	return l
}

func makeUnion(l1, l2 *Lens) *Lens {
	l := &Lens{tag: lUnion, children: []*Lens{l1, l2}}
	l.ctype = regexpUnionN([]*Regexp{l1.ctype, l2.ctype})
	l.consumesValue = l1.consumesValue && l2.consumesValue
	l.recursive = l1.recursive || l2.recursive
	return l
}

func makeSubtree(c *Lens) *Lens {
	l := &Lens{tag: lSubtree, child: c}
	l.ctype = c.ctype
	l.recursive = c.recursive
	return l
}

func makeStar(c *Lens) *Lens {
	l := &Lens{tag: lStar, child: c}
	l.ctype = regexpIter(c.ctype, 0, -1)
	l.recursive = c.recursive
	return l
}

func makePlus(c *Lens) *Lens {
	return makeConcat(c, makeStar(c))
}

func makeMaybe(c *Lens) *Lens {
	l := &Lens{tag: lMaybe, child: c}
	l.ctype = regexpMaybe(c.ctype)
	l.key = c.key
	l.value = c.value
	l.recursive = c.recursive
	return l
}

// makeSquare builds square l1 l2 l3, which matches l1 . l2 . l3 where the text
// matched by l1 and l3 must be identical (a balanced delimiter). Its child is a
// flat 3-way concat, matching how upstream get_square walks the registers.
func makeSquare(l1, l2, l3 *Lens) *Lens {
	inner := &Lens{tag: lConcat, children: []*Lens{l1, l2, l3}}
	inner.ctype = regexpConcatN([]*Regexp{l1.ctype, l2.ctype, l3.ctype})
	inner.consumesValue = l1.consumesValue || l2.consumesValue || l3.consumesValue
	inner.recursive = l1.recursive || l2.recursive || l3.recursive
	l := &Lens{tag: lSquare, child: inner}
	l.ctype = inner.ctype
	l.consumesValue = inner.consumesValue
	l.recursive = inner.recursive
	return l
}

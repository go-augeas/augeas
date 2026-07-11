// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"strconv"
	"strings"
)

// getState mirrors upstream struct state for the get direction. regs is a
// FindSubmatchIndex-style array (2 ints per group; -1 when a group did not
// participate). nreg selects the group describing the substring currently being
// processed, exactly as in get.c.
type getState struct {
	text string
	regs []int
	nreg int
	key  *string
	val  *string
	seqs map[string]int
	err  error
}

func (s *getState) regStart() int { return s.regs[2*s.nreg] }
func (s *getState) regEnd() int   { return s.regs[2*s.nreg+1] }

func (s *getState) regValid() bool {
	return s.regs != nil && 2*s.nreg+1 < len(s.regs)
}

func (s *getState) regMatched() bool {
	return s.regValid() && s.regs[2*s.nreg] >= 0
}

func (s *getState) fail(format string, a ...any) {
	if s.err == nil {
		s.err = fmt.Errorf(format, a...)
	}
}

// LnsGet parses text with lens, returning the resulting forest. It reproduces
// lns_get: it first matches the whole text (or, for a top-level star, treats
// the whole text as the star's span), then walks the lens tree driven by the
// register array.
func LnsGet(lens *Lens, text string) ([]*Tree, error) {
	if lens.recursive {
		return getRec(lens, text)
	}
	s := &getState{text: text, seqs: map[string]int{}}
	size := len(text)
	partial, err := initRegs(s, lens, size)
	if err != nil {
		return nil, err
	}
	forest := getLens(lens, s, true)
	if s.err != nil {
		return nil, s.err
	}
	if s.key != nil {
		return nil, fmt.Errorf("get left unused key %q", *s.key)
	}
	if s.val != nil {
		return nil, fmt.Errorf("get left unused value %q", *s.val)
	}
	if partial {
		return nil, fmt.Errorf("get did not match entire input")
	}
	return forest, nil
}

func initRegs(s *getState, lens *Lens, size int) (bool, error) {
	if lens.tag != lStar && !lens.recursive {
		regs, matched, err := lens.ctype.match(s.text, 0, size)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, fmt.Errorf("input string does not match at all")
		}
		s.regs = regs
		s.nreg = 0
		return regs[1] != size, nil
	}
	s.regs = []int{0, size}
	s.nreg = 0
	return false, nil
}

// getLens walks lens l over the register array. wrapped reports whether s.nreg
// currently points at l's own capturing wrapper group — true when a parent
// concat/union captured l (leaf, union branch, square component) or l is the
// whole match; false when a parent concat spliced l in structurally without a
// wrapper. Composites skip their wrapper (s.nreg++) only when wrapped; leaves
// read it directly.
func getLens(l *Lens, s *getState, wrapped bool) []*Tree {
	if s.err != nil {
		return nil
	}
	switch l.tag {
	case lDel:
		if !s.regMatched() {
			s.fail("no match for del %s", l.ctype)
		}
		return nil
	case lStore:
		if !s.regMatched() {
			s.fail("no match for store %s", l.ctype)
		} else {
			s.val = strptr(s.text[s.regStart():s.regEnd()])
		}
		return nil
	case lKey:
		if !s.regMatched() {
			s.fail("no match for key %s", l.ctype)
		} else {
			s.key = strptr(s.text[s.regStart():s.regEnd()])
		}
		return nil
	case lLabel:
		s.key = strptr(l.str)
		return nil
	case lValue:
		s.val = strptr(l.str)
		return nil
	case lSeq:
		v := s.seqs[l.str]
		if v == 0 {
			v = 1
		}
		s.key = strptr(strconv.Itoa(v))
		s.seqs[l.str] = v + 1
		return nil
	case lCounter:
		s.seqs[l.str] = 1
		return nil
	case lConcat:
		return getConcat(l, s, wrapped)
	case lUnion:
		return getUnion(l, s, wrapped)
	case lSubtree:
		return getSubtree(l, s, wrapped)
	case lStar:
		return getStar(l, s)
	case lMaybe:
		return getMaybe(l, s, wrapped)
	case lSquare:
		return getSquare(l, s)
	default:
		s.fail("unsupported lens tag %d in get", l.tag)
		return nil
	}
}

func getConcat(l *Lens, s *getState, wrapped bool) []*Tree {
	var out []*Tree
	oldNreg := s.nreg
	if wrapped {
		s.nreg++
	}
	for _, c := range l.children {
		cap := l.fullCapture || ctypeCaptures(c)
		// Only a capturing child indexes s.nreg into the register array; a
		// structural (non-capturing) child may legitimately leave s.nreg past
		// the last group, so the range check applies only when cap.
		if cap && !s.regValid() {
			s.fail("not enough components in concat")
			s.nreg = oldNreg
			return nil
		}
		out = append(out, getLens(c, s, cap)...)
		if cap {
			s.nreg++
		}
		s.nreg += c.ctype.nsub()
	}
	s.nreg = oldNreg
	return out
}

func getUnion(l *Lens, s *getState, wrapped bool) []*Tree {
	oldNreg := s.nreg
	if wrapped {
		s.nreg++
	}
	var out []*Tree
	applied := false
	for _, c := range l.children {
		if s.regMatched() {
			out = getLens(c, s, true)
			applied = true
			break
		}
		s.nreg += 1 + c.ctype.nsub()
	}
	s.nreg = oldNreg
	if !applied {
		s.fail("no matching branch in union")
	}
	return out
}

func getSubtree(l *Lens, s *getState, wrapped bool) []*Tree {
	key := s.key
	val := s.val
	s.key = nil
	s.val = nil
	children := getLens(l.child, s, wrapped)
	node := &Tree{Label: s.key, Value: s.val, Children: children}
	s.key = key
	s.val = val
	return []*Tree{node}
}

func getStar(l *Lens, s *getState) []*Tree {
	start := s.regStart()
	end := s.regEnd()
	size := end - start

	oldRegs := s.regs
	oldNreg := s.nreg
	s.regs = nil
	s.nreg = 0

	var out []*Tree
	for size > 0 {
		regs, matched, err := l.child.ctype.match(s.text, start, end)
		if err != nil {
			s.fail("%v", err)
			break
		}
		if !matched {
			break
		}
		count := regs[1] - start
		if count <= 0 {
			break
		}
		s.regs = regs
		s.nreg = 0
		out = append(out, getLens(l.child, s, true)...)
		start += count
		size -= count
	}

	s.regs = oldRegs
	s.nreg = oldNreg
	if size != 0 {
		s.fail("iterated lens matched less than it should")
	}
	return out
}

func getMaybe(l *Lens, s *getState, wrapped bool) []*Tree {
	if wrapped {
		s.nreg++
	}
	var out []*Tree
	if s.regMatched() {
		out = getLens(l.child, s, true)
	}
	if wrapped {
		s.nreg--
	}
	return out
}

func getSquare(l *Lens, s *getState) []*Tree {
	concat := l.child
	start := s.regStart()
	end := s.regEnd()

	oldRegs := s.regs
	oldNreg := s.nreg
	s.regs = nil
	s.nreg = 0

	regs, matched, err := concat.ctype.match(s.text, start, end)
	if err != nil {
		s.regs = oldRegs
		s.nreg = oldNreg
		s.fail("%v", err)
		return nil
	}
	if !matched {
		s.regs = oldRegs
		s.nreg = oldNreg
		s.fail("no match for square")
		return nil
	}
	s.regs = regs
	s.nreg = 0
	out := getLens(concat, s, true)

	// left component is the first child of the concat (nreg = 1)
	s.nreg = 1
	lsqr := s.text[s.regStart():s.regEnd()]
	// advance to the last child
	for i := 0; i < len(concat.children)-1; i++ {
		s.nreg += 1 + concat.children[i].ctype.nsub()
	}
	rsqr := s.text[s.regStart():s.regEnd()]

	nocase := concat.children[0].ctype.nocase || concat.children[len(concat.children)-1].ctype.nocase
	eq := lsqr == rsqr
	if nocase {
		eq = strings.EqualFold(lsqr, rsqr)
	}

	s.regs = oldRegs
	s.nreg = oldNreg
	if !eq {
		s.fail("square mismatch: %q vs %q", lsqr, rsqr)
		return nil
	}
	return out
}

// recResult is one possible parse of a (sub)lens starting at a position: how
// far it consumed, the forest it produced, and any pending key/value that a
// surrounding subtree will absorb.
type recResult struct {
	end   int
	trees []*Tree
	key   *string
	val   *string
	skel  *skel  // skeleton for put
	dict  *pdict // dictionary for put
}

type recState struct {
	text string
	memo map[recKey][]recResult
	busy map[recKey]bool
	seqs map[string]int
	err  error
}

type recKey struct {
	l   *Lens
	pos int
}

// getRec parses a recursive lens by a memoised recursive-descent chart parse.
// Corpus recursive lenses are guarded (they consume a delimiter before
// recursing), so left-recursion cycles are broken by the busy guard without
// losing valid parses. It selects the parse that consumes the whole input.
func getRec(lens *Lens, text string) ([]*Tree, error) {
	s := &recState{
		text: text,
		memo: map[recKey][]recResult{},
		busy: map[recKey]bool{},
		seqs: map[string]int{},
	}
	results := s.parse(lens, 0)
	if s.err != nil {
		return nil, s.err
	}
	for _, r := range results {
		if r.end == len(text) {
			return r.trees, nil
		}
	}
	return nil, fmt.Errorf("recursive get did not match entire input")
}

func (s *recState) parse(l *Lens, pos int) []recResult {
	key := recKey{l, pos}
	if r, ok := s.memo[key]; ok {
		return r
	}
	if s.busy[key] {
		return nil // break left-recursion cycle
	}
	s.busy[key] = true
	res := dedupByEnd(s.parseUncached(l, pos))
	s.busy[key] = false
	s.memo[key] = res
	return res
}

// dedupByEnd keeps at most one result per distinct end position. The corpus
// lenses are unambiguous, so this preserves correctness while preventing the
// exponential result fan-out that a naive chart parse would produce on large
// inputs.
func dedupByEnd(rs []recResult) []recResult {
	if len(rs) <= 1 {
		return rs
	}
	seen := make(map[int]bool, len(rs))
	out := rs[:0:0]
	for _, r := range rs {
		if seen[r.end] {
			continue
		}
		seen[r.end] = true
		out = append(out, r)
	}
	return out
}

func (s *recState) parseUncached(l *Lens, pos int) []recResult {
	// A non-recursive sub-lens is a "terminal" of the recursive grammar: match
	// its whole ctype and build its tree with the coordinated single-match get,
	// which correctly resolves internal boundaries (e.g. a store followed by a
	// delimiter) that naive per-leaf longest matching would get wrong.
	// Every non-recursive sub-lens (all leaves included) is handled by the
	// terminal fast path, so the switch below only sees recursive structural
	// nodes.
	if !l.recursive && l.tag != lRec {
		return s.parseTerminal(l, pos)
	}
	switch l.tag {
	case lRec:
		return s.parse(l.body, pos)
	case lSubtree:
		var out []recResult
		for _, cr := range s.parse(l.child, pos) {
			node := &Tree{Label: cr.key, Value: cr.val, Children: cr.trees}
			r := recResult{end: cr.end, trees: []*Tree{node}, skel: &skel{tag: lSubtree}}
			r.dict = makeDict(cr.key, cr.skel, cr.dict)
			out = append(out, r)
		}
		return out
	case lMaybe:
		out := append([]recResult{}, s.parse(l.child, pos)...)
		return append(out, recResult{end: pos, skel: &skel{tag: lMaybe}})
	case lUnion:
		var out []recResult
		for _, c := range l.children {
			out = append(out, s.parse(c, pos)...)
		}
		return out
	case lConcat:
		return s.parseRecConcat(l.children, pos)
	case lStar:
		return s.parseStar(l.child, pos)
	case lSquare:
		concat := l.child
		l1, l2, l3 := concat.children[0], concat.children[1], concat.children[2]
		nocase := l1.ctype.nocase || l3.ctype.nocase
		var out []recResult
		for _, r1 := range s.parse(l1, pos) {
			lsq := s.text[pos:r1.end]
			for _, r2 := range s.parse(l2, r1.end) {
				for _, r3 := range s.parse(l3, r2.end) {
					rsq := s.text[r2.end:r3.end]
					eq := lsq == rsq
					if nocase {
						eq = strings.EqualFold(lsq, rsq)
					}
					if eq {
						m := mergeConcat(mergeConcat(r1, r2), r3)
						m.skel = &skel{tag: lSquare, skels: []*skel{{tag: lConcat, skels: []*skel{r1.skel, r2.skel, r3.skel}}}}
						m.dict = dictMerge(dictMerge(r1.dict, r2.dict), r3.dict)
						out = append(out, m)
					}
				}
			}
		}
		return out
	default:
		if s.err == nil {
			s.err = fmt.Errorf("unsupported lens tag %d in recursive get", l.tag)
		}
		return nil
	}
}

// parseTerminal matches a non-recursive sub-lens's whole ctype at pos and
// builds its tree via the coordinated single-match get.
func (s *recState) parseTerminal(l *Lens, pos int) []recResult {
	regs, matched, err := l.ctype.match(s.text, pos, len(s.text))
	if err != nil {
		if s.err == nil {
			s.err = err
		}
		return nil
	}
	if !matched {
		return nil
	}
	gs := &getState{text: s.text, regs: regs, nreg: 0, seqs: s.seqs}
	trees := getLens(l, gs, true)
	if gs.err != nil {
		return nil
	}
	// Build the skeleton/dictionary for the same span so put can reuse it.
	ps := &getState{text: s.text, regs: regs, nreg: 0, seqs: map[string]int{}}
	sk, d := parseLens(l, ps, true)
	return []recResult{{end: regs[1], trees: trees, key: gs.key, val: gs.val, skel: sk, dict: d}}
}

func (s *recState) parseStar(child *Lens, pos int) []recResult {
	// zero or more repetitions; collect all reachable end positions.
	empty := recResult{end: pos, skel: &skel{tag: lStar}}
	results := []recResult{empty}
	out := []recResult{empty}
	for len(results) > 0 {
		var next []recResult
		for _, r := range results {
			for _, cr := range s.parse(child, r.end) {
				if cr.end <= r.end { // no progress
					continue
				}
				m := mergeConcat(r, cr)
				m.skel = &skel{tag: lStar, skels: append(append([]*skel{}, r.skel.skels...), cr.skel)}
				m.dict = dictMerge(r.dict, cr.dict)
				next = append(next, m)
				out = append(out, m)
			}
		}
		results = dedupByEnd(next)
	}
	return out
}

// parseRecConcat parses a concat within a recursive lens, building a concat
// skeleton and merged dictionary.
func (s *recState) parseRecConcat(children []*Lens, pos int) []recResult {
	type acc struct {
		r     recResult
		skels []*skel
		dict  *pdict
	}
	accs := []acc{{r: recResult{end: pos}}}
	for _, c := range children {
		var next []acc
		for _, a := range accs {
			for _, cr := range s.parse(c, a.r.end) {
				m := mergeConcat(a.r, cr)
				ns := append(append([]*skel{}, a.skels...), cr.skel)
				nd := dictMerge(a.dict, cr.dict)
				next = append(next, acc{r: m, skels: ns, dict: nd})
			}
		}
		// dedup by end position to keep the parse polynomial
		seen := map[int]bool{}
		accs = accs[:0]
		for _, a := range next {
			if seen[a.r.end] {
				continue
			}
			seen[a.r.end] = true
			accs = append(accs, a)
		}
		if len(accs) == 0 {
			return nil
		}
	}
	out := make([]recResult, 0, len(accs))
	for _, a := range accs {
		a.r.skel = &skel{tag: lConcat, skels: a.skels}
		a.r.dict = a.dict
		out = append(out, a.r)
	}
	return out
}

func mergeConcat(a, b recResult) recResult {
	r := recResult{end: b.end}
	r.trees = append(append([]*Tree{}, a.trees...), b.trees...)
	r.key = a.key
	if r.key == nil {
		r.key = b.key
	}
	r.val = a.val
	if r.val == nil {
		r.val = b.val
	}
	return r
}

// dictMerge returns a new dictionary with b's entries appended after a's,
// without mutating either operand (results are shared via memoisation).
func dictMerge(a, b *pdict) *pdict {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	m := &pdict{entries: make(map[string][]*dentry, len(a.entries)+len(b.entries)), idx: map[string]int{}}
	for k, v := range a.entries {
		m.entries[k] = append([]*dentry{}, v...)
	}
	for k, v := range b.entries {
		m.entries[k] = append(m.entries[k], v...)
	}
	return m
}

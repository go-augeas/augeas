// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"strconv"
	"strings"
)

// skel is the skeleton of the original text produced by parsing: it records the
// del/key/store tokens so an unchanged tree can be re-serialised verbatim.
type skel struct {
	tag   lensTag
	text  string
	skels []*skel
}

// dentry is one (skeleton, sub-dictionary) pair stored under a label.
type dentry struct {
	skel *skel
	dict *pdict
}

// pdict maps a subtree label to the skeletons parsed for it, consumed FIFO so
// that reordered same-labelled nodes reuse their original text.
type pdict struct {
	entries map[string][]*dentry
	idx     map[string]int
}

const dictNil = "\x00\x00nil"

func dictKey(label *string) string {
	if label == nil {
		return dictNil
	}
	return *label
}

func makeDict(label *string, sk *skel, sub *pdict) *pdict {
	return &pdict{
		entries: map[string][]*dentry{dictKey(label): {{skel: sk, dict: sub}}},
		idx:     map[string]int{},
	}
}

func dictAppend(d1, d2 *pdict) *pdict {
	if d2 == nil {
		return d1
	}
	if d1 == nil {
		return d2
	}
	for k, es := range d2.entries {
		d1.entries[k] = append(d1.entries[k], es...)
	}
	return d1
}

func (d *pdict) lookup(label *string) (*skel, *pdict) {
	if d == nil {
		return nil, nil
	}
	k := dictKey(label)
	i := d.idx[k]
	es := d.entries[k]
	if i >= len(es) {
		return nil, nil
	}
	d.idx[k] = i + 1
	return es[i].skel, es[i].dict
}

// ---- parse phase: build skeleton + dictionary (mirrors the get walk) ----

func lnsParse(lens *Lens, text string) (*skel, *pdict, error) {
	if lens.recursive {
		return parseRecTop(lens, text)
	}
	s := &getState{text: text, seqs: map[string]int{}}
	if _, err := initRegs(s, lens, len(text)); err != nil {
		return nil, nil, err
	}
	sk, d := parseLens(lens, s)
	if s.err != nil {
		return nil, nil, s.err
	}
	return sk, d, nil
}

func parseLens(l *Lens, s *getState) (*skel, *pdict) {
	if s.err != nil {
		return nil, nil
	}
	switch l.tag {
	case lDel:
		if !s.regMatched() {
			s.fail("no match for del in parse")
			return &skel{tag: lDel}, nil
		}
		return &skel{tag: lDel, text: s.text[s.regStart():s.regEnd()]}, nil
	case lStore:
		return &skel{tag: lStore}, nil
	case lKey:
		if s.regMatched() {
			s.key = strptr(s.text[s.regStart():s.regEnd()])
		}
		return &skel{tag: lKey}, nil
	case lLabel:
		s.key = strptr(l.str)
		return &skel{tag: lLabel}, nil
	case lValue:
		return &skel{tag: lValue}, nil
	case lSeq:
		v := s.seqs[l.str]
		if v == 0 {
			v = 1
		}
		s.key = strptr(strconv.Itoa(v))
		s.seqs[l.str] = v + 1
		return &skel{tag: lSeq}, nil
	case lCounter:
		s.seqs[l.str] = 1
		return &skel{tag: lCounter}, nil
	case lConcat:
		return parseConcat(l, s)
	case lUnion:
		return parseUnion(l, s)
	case lSubtree:
		return parseSubtree(l, s)
	case lStar:
		return parseStar(l, s)
	case lMaybe:
		return parseMaybe(l, s)
	case lSquare:
		return parseSquare(l, s)
	}
	s.fail("unsupported lens tag %d in parse", l.tag)
	return nil, nil
}

func parseConcat(l *Lens, s *getState) (*skel, *pdict) {
	sk := &skel{tag: lConcat}
	var d *pdict
	old := s.nreg
	s.nreg++
	for _, c := range l.children {
		if !s.regValid() {
			s.fail("not enough components in concat")
			s.nreg = old
			return sk, d
		}
		cs, cd := parseLens(c, s)
		sk.skels = append(sk.skels, cs)
		d = dictAppend(d, cd)
		s.nreg += 1 + c.ctype.nsub()
	}
	s.nreg = old
	return sk, d
}

func parseUnion(l *Lens, s *getState) (*skel, *pdict) {
	old := s.nreg
	s.nreg++
	var sk *skel
	var d *pdict
	applied := false
	for _, c := range l.children {
		if s.regMatched() {
			sk, d = parseLens(c, s)
			applied = true
			break
		}
		s.nreg += 1 + c.ctype.nsub()
	}
	s.nreg = old
	if !applied {
		s.fail("no matching branch in union (parse)")
	}
	return sk, d
}

func parseSubtree(l *Lens, s *getState) (*skel, *pdict) {
	key := s.key
	s.key = nil
	cs, cd := parseLens(l.child, s)
	d := makeDict(s.key, cs, cd)
	s.key = key
	return &skel{tag: lSubtree}, d
}

func parseStar(l *Lens, s *getState) (*skel, *pdict) {
	sk := &skel{tag: lStar}
	var d *pdict
	start := s.regStart()
	end := s.regEnd()
	size := end - start
	oldRegs := s.regs
	oldNreg := s.nreg
	s.regs = nil
	s.nreg = 0
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
		cs, cd := parseLens(l.child, s)
		sk.skels = append(sk.skels, cs)
		d = dictAppend(d, cd)
		start += count
		size -= count
	}
	s.regs = oldRegs
	s.nreg = oldNreg
	return sk, d
}

func parseMaybe(l *Lens, s *getState) (*skel, *pdict) {
	s.nreg++
	defer func() { s.nreg-- }()
	if s.regMatched() {
		return parseLens(l.child, s)
	}
	return &skel{tag: lMaybe}, nil
}

func parseSquare(l *Lens, s *getState) (*skel, *pdict) {
	concat := l.child
	start := s.regStart()
	end := s.regEnd()
	oldRegs := s.regs
	oldNreg := s.nreg
	s.regs = nil
	s.nreg = 0
	regs, matched, err := concat.ctype.match(s.text, start, end)
	if err != nil || !matched {
		s.regs = oldRegs
		s.nreg = oldNreg
		s.fail("no match for square (parse)")
		return &skel{tag: lSquare}, nil
	}
	s.regs = regs
	s.nreg = 0
	cs, cd := parseLens(concat, s)
	s.regs = oldRegs
	s.nreg = oldNreg
	return &skel{tag: lSquare, skels: []*skel{cs}}, cd
}

func parseRecTop(lens *Lens, text string) (*skel, *pdict, error) {
	s := &recState{
		text: text,
		memo: map[recKey][]recResult{},
		busy: map[recKey]bool{},
		seqs: map[string]int{},
	}
	results := s.parse(lens, 0)
	if s.err != nil {
		return nil, nil, s.err
	}
	for _, r := range results {
		if r.end == len(text) {
			return r.skel, r.dict, nil
		}
	}
	return nil, nil, fmt.Errorf("recursive parse did not match entire input")
}

// ---- put phase ----

type psplit struct {
	nodes []*Tree
	enc   string
}

type putState struct {
	out      *strings.Builder
	tree     *Tree
	split    *psplit
	skel     *skel
	dict     *pdict
	override *string
	err      error
}

func (s *putState) fail(format string, a ...any) {
	if s.err == nil {
		s.err = fmt.Errorf(format, a...)
	}
}

// encStr encodes a possibly-nil string as its bytes (nil -> empty).
func encStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func encodeForest(forest []*Tree) string {
	var b strings.Builder
	for _, t := range forest {
		b.WriteString(encStr(t.Label))
		b.WriteString(encEq)
		b.WriteString(encStr(t.Value))
		b.WriteString(encSlash)
	}
	return b.String()
}

func countSlash(enc string, upto int) int {
	n := 0
	for i := 0; i < upto && i < len(enc); i++ {
		if enc[i] == encSlash[0] {
			n++
		}
	}
	return n
}

// LnsPut serialises forest back to text using lens, reusing the skeleton parsed
// from the original text where the tree is unchanged.
func LnsPut(lens *Lens, forest []*Tree, text string) (string, error) {
	sk, dict, err := lnsParse(lens, text)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	s := &putState{
		out:   &out,
		skel:  sk,
		dict:  dict,
		split: &psplit{nodes: forest, enc: encodeForest(forest)},
	}
	putLens(lens, s)
	if s.err != nil {
		return "", s.err
	}
	return out.String(), nil
}

func (s *putState) emit(text string) { s.out.WriteString(text) }

func splitConcat(sp *psplit, lens *Lens) ([]*psplit, error) {
	regs, ok, err := lens.atype.match(sp.enc, 0, len(sp.enc))
	if err != nil {
		return nil, err
	}
	if !ok || regs[1] != len(sp.enc) {
		return nil, fmt.Errorf("tree does not match lens schema (concat)")
	}
	out := make([]*psplit, 0, len(lens.children))
	reg := 1
	for _, c := range lens.children {
		if 2*reg+1 >= len(regs) {
			return nil, fmt.Errorf("concat split register out of range")
		}
		gs, ge := regs[2*reg], regs[2*reg+1]
		if gs < 0 {
			return nil, fmt.Errorf("unmatched group in concat split")
		}
		ci := countSlash(sp.enc, gs)
		cj := countSlash(sp.enc, ge)
		if ci > cj || cj > len(sp.nodes) {
			return nil, fmt.Errorf("concat split node range out of bounds")
		}
		out = append(out, &psplit{nodes: sp.nodes[ci:cj], enc: sp.enc[gs:ge]})
		reg += 1 + c.atype.nsub()
	}
	return out, nil
}

func splitIter(sp *psplit, child *Lens) ([]*psplit, error) {
	var out []*psplit
	pos := 0
	for pos < len(sp.enc) {
		regs, ok, err := child.atype.match(sp.enc, pos, len(sp.enc))
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		count := regs[1] - pos
		if count <= 0 {
			break
		}
		// count > 0 with monotonic countSlash guarantees ci <= cj <= len(nodes).
		ci := countSlash(sp.enc, pos)
		cj := countSlash(sp.enc, pos+count)
		out = append(out, &psplit{nodes: sp.nodes[ci:cj], enc: sp.enc[pos : pos+count]})
		pos += count
	}
	return out, nil
}

func applies(lens *Lens, s *putState) bool {
	regs, ok, err := lens.atype.match(s.split.enc, 0, len(s.split.enc))
	if err != nil || !ok {
		return false
	}
	if regs[1] != len(s.split.enc) {
		return false
	}
	if regs[1] == 0 && lens.value {
		return s.tree != nil && s.tree.Value != nil
	}
	return true
}

func skelInstanceOf(lens *Lens, sk *skel) bool {
	if sk == nil {
		return false
	}
	switch lens.tag {
	case lDel:
		if sk.tag != lDel {
			return false
		}
		regs, ok, err := lens.regexp.match(sk.text, 0, len(sk.text))
		return err == nil && ok && regs[1] == len(sk.text)
	case lStore:
		return sk.tag == lStore
	case lKey:
		return sk.tag == lKey
	case lLabel:
		return sk.tag == lLabel
	case lValue:
		return sk.tag == lValue
	case lSeq:
		return sk.tag == lSeq
	case lCounter:
		return sk.tag == lCounter
	case lConcat:
		if sk.tag != lConcat || len(sk.skels) != len(lens.children) {
			return false
		}
		for i, c := range lens.children {
			if !skelInstanceOf(c, sk.skels[i]) {
				return false
			}
		}
		return true
	case lUnion:
		for _, c := range lens.children {
			if skelInstanceOf(c, sk) {
				return true
			}
		}
		return false
	case lSubtree:
		return sk.tag == lSubtree
	case lMaybe:
		return sk.tag == lMaybe || skelInstanceOf(lens.child, sk)
	case lStar:
		if sk.tag != lStar {
			return false
		}
		for _, cs := range sk.skels {
			if !skelInstanceOf(lens.child, cs) {
				return false
			}
		}
		return true
	case lRec:
		return skelInstanceOf(lens.body, sk)
	case lSquare:
		return sk.tag == lSquare && len(sk.skels) == 1 && skelInstanceOf(lens.child, sk.skels[0])
	}
	return false
}

func putLens(l *Lens, s *putState) {
	if s.err != nil {
		return
	}
	switch l.tag {
	case lDel:
		if s.override != nil {
			s.emit(*s.override)
		} else if s.skel != nil {
			s.emit(s.skel.text)
		}
	case lStore:
		putStore(l, s)
	case lKey:
		s.emit(encStr(s.tree.Label))
	case lLabel, lValue, lSeq, lCounter:
		// nothing to emit
	case lConcat:
		putConcat(l, s)
	case lUnion:
		putUnion(l, s)
	case lSubtree:
		putSubtree(l, s)
	case lStar:
		putStar(l, s)
	case lMaybe:
		putMaybe(l, s)
	case lSquare:
		putSquare(l, s)
	case lRec:
		putLens(l.body, s)
	default:
		s.fail("unsupported lens tag %d in put", l.tag)
	}
}

func putStore(l *Lens, s *putState) {
	if s.tree == nil || s.tree.Value == nil {
		s.fail("cannot store a nonexistent value")
		return
	}
	v := *s.tree.Value
	re := l.regexp
	if re != nil {
		regs, ok, err := re.match(v, 0, len(v))
		if err != nil || !ok || regs[1] != len(v) {
			s.fail("value %q does not match store regexp %s", v, re)
			return
		}
	}
	s.emit(v)
}

func putConcat(l *Lens, s *putState) {
	oldSplit := s.split
	oldSkel := s.skel
	splits, err := splitConcat(oldSplit, l)
	if err != nil {
		s.fail("%v", err)
		return
	}
	skels := s.skel.skels
	for i, c := range l.children {
		s.split = splits[i]
		if i < len(skels) {
			s.skel = skels[i]
		} else {
			s.skel = nil
		}
		putLens(c, s)
	}
	s.split = oldSplit
	s.skel = oldSkel
}

func putUnion(l *Lens, s *putState) {
	for _, c := range l.children {
		if applies(c, s) {
			if skelInstanceOf(c, s.skel) {
				putLens(c, s)
			} else {
				createLens(c, s)
			}
			return
		}
	}
	s.fail("none of the union alternatives match the tree")
}

func putSubtree(l *Lens, s *putState) {
	if len(s.split.nodes) == 0 {
		s.fail("subtree with no matching node")
		return
	}
	node := s.split.nodes[0]
	oldTree := s.tree
	oldSplit := s.split
	oldSkel := s.skel
	oldDict := s.dict

	s.tree = node
	s.split = &psplit{nodes: node.Children, enc: encodeForest(node.Children)}
	childSkel, childDict := oldDict.lookup(node.Label)
	s.skel = childSkel
	s.dict = childDict
	if childSkel == nil || !skelInstanceOf(l.child, childSkel) {
		createLens(l.child, s)
	} else {
		putLens(l.child, s)
	}

	s.tree = oldTree
	s.split = oldSplit
	s.skel = oldSkel
	s.dict = oldDict
}

func putStar(l *Lens, s *putState) {
	oldSplit := s.split
	oldSkel := s.skel
	splits, err := splitIter(oldSplit, l.child)
	if err != nil {
		s.fail("%v", err)
		return
	}
	if s.skel == nil {
		for _, sp := range splits {
			s.split = sp
			createLens(l.child, s)
		}
		s.split = oldSplit
		return
	}
	skels := s.skel.skels
	i := 0
	for ; i < len(splits) && i < len(skels); i++ {
		s.split = splits[i]
		s.skel = skels[i]
		putLens(l.child, s)
	}
	for ; i < len(splits); i++ {
		s.split = splits[i]
		s.skel = nil
		createLens(l.child, s)
	}
	s.split = oldSplit
	s.skel = oldSkel
}

func putMaybe(l *Lens, s *putState) {
	child := l.child
	if applies(child, s) {
		if skelInstanceOf(child, s.skel) {
			putLens(child, s)
		} else {
			createLens(child, s)
		}
	}
}

func putSquare(l *Lens, s *putState) {
	oldSplit := s.split
	oldSkel := s.skel
	concat := l.child
	left := concat.children[0]
	splits, err := splitConcat(oldSplit, concat)
	if err != nil {
		s.fail("%v", err)
		return
	}
	var skels []*skel
	if s.skel != nil && len(s.skel.skels) == 1 {
		skels = s.skel.skels[0].skels
	}
	for i, c := range concat.children {
		s.split = splits[i]
		if i < len(skels) {
			s.skel = skels[i]
		} else {
			s.skel = nil
		}
		if i == len(concat.children)-1 && left.tag == lKey {
			s.override = s.tree.Label
		}
		putLens(c, s)
		s.override = nil
	}
	s.split = oldSplit
	s.skel = oldSkel
}

// ---- create phase (fresh subtree, no reusable skeleton) ----

func createLens(l *Lens, s *putState) {
	if s.err != nil {
		return
	}
	switch l.tag {
	case lDel:
		if s.override != nil {
			s.emit(*s.override)
		} else {
			s.emit(l.str)
		}
	case lStore:
		putStore(l, s)
	case lKey:
		s.emit(encStr(s.tree.Label))
	case lLabel, lValue, lSeq, lCounter:
		// nothing
	case lConcat:
		createConcat(l, s)
	case lUnion:
		createUnion(l, s)
	case lSubtree:
		putSubtree(l, s)
	case lStar:
		createStar(l, s)
	case lMaybe:
		if applies(l.child, s) {
			createLens(l.child, s)
		}
	case lSquare:
		createSquare(l, s)
	case lRec:
		createLens(l.body, s)
	default:
		s.fail("unsupported lens tag %d in create", l.tag)
	}
}

func createConcat(l *Lens, s *putState) {
	oldSplit := s.split
	splits, err := splitConcat(oldSplit, l)
	if err != nil {
		s.fail("%v", err)
		return
	}
	for i, c := range l.children {
		s.split = splits[i]
		createLens(c, s)
	}
	s.split = oldSplit
}

func createUnion(l *Lens, s *putState) {
	for _, c := range l.children {
		if applies(c, s) {
			createLens(c, s)
			return
		}
	}
	s.fail("none of the union alternatives match the tree (create)")
}

func createStar(l *Lens, s *putState) {
	oldSplit := s.split
	splits, err := splitIter(oldSplit, l.child)
	if err != nil {
		s.fail("%v", err)
		return
	}
	for _, sp := range splits {
		s.split = sp
		createLens(l.child, s)
	}
	s.split = oldSplit
}

func createSquare(l *Lens, s *putState) {
	oldSplit := s.split
	concat := l.child
	left := concat.children[0]
	splits, err := splitConcat(oldSplit, concat)
	if err != nil {
		s.fail("%v", err)
		return
	}
	for i, c := range concat.children {
		s.split = splits[i]
		if i == len(concat.children)-1 && left.tag == lKey {
			s.override = s.tree.Label
		}
		createLens(c, s)
		s.override = nil
	}
	s.split = oldSplit
}

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"errors"
	"strings"
	"testing"
)

// These are direct unit tests of internal put/get/dfa helpers, exercising the
// defensive branches that are unreachable through normal end-to-end flows.

func TestGetLensErrGuard(t *testing.T) {
	s := &getState{err: errors.New("preset"), regs: []int{0, 0}}
	if got := getLens(&Lens{tag: lDel, ctype: regexpMakeEmpty()}, s, true); got != nil {
		t.Error("getLens should return nil when s.err is set")
	}
}

func TestReUnionIdentical(t *testing.T) {
	a, b := "x", "x"
	if reUnion(&a, &b) != &a {
		t.Error("reUnion of identical operands should return the first")
	}
}

func TestSkelInstanceOfStarChild(t *testing.T) {
	star := &Lens{tag: lStar, child: makePrim(lDel, newRegexp("a", false), "a")}
	// star skeleton whose element skeleton is not a del -> child mismatch
	sk := &skel{tag: lStar, skels: []*skel{{tag: lStore}}}
	if skelInstanceOf(star, sk) {
		t.Error("skelInstanceOf should reject a star whose child skel mismatches")
	}
}

func TestPutSubtreeNoNodeDirect(t *testing.T) {
	var out strings.Builder
	s := &putState{out: &out, split: &psplit{nodes: nil, enc: ""}, dict: nil}
	putSubtree(&Lens{tag: lSubtree, child: makePrim(lStore, newRegexp("a", false), ""), atype: regexpMakeEmpty()}, s)
	if s.err == nil {
		t.Error("putSubtree with no node should fail")
	}
}

func TestPutStarNilAndShortSkel(t *testing.T) {
	l := compileLens(t, ` let lns = ( [ key /[a-z]+/ . del /=/ "=" . store /[0-9]+/ ] . del /\n/ "\n" )*`)
	forest, err := LnsGet(l, "a=1\nb=2\n")
	if err != nil {
		t.Fatal(err)
	}
	enc := encodeForest(forest)
	var out strings.Builder
	// nil skel -> create-all branch in putStar
	s := &putState{out: &out, split: &psplit{nodes: forest, enc: enc}, skel: nil}
	safe(func() { putStar(l, s) })
	// short skel (fewer elements than splits) -> the create-extra loop
	var out2 strings.Builder
	s2 := &putState{out: &out2, split: &psplit{nodes: forest, enc: enc}, skel: &skel{tag: lStar}}
	safe(func() { putStar(l, s2) })
}

func TestPutConcatShortSkel(t *testing.T) {
	l := compileLens(t, ` let lns = [ key /[a-z]+/ . del /=/ "=" . store /[0-9]+/ ]`)
	forest, err := LnsGet(l, "a=1")
	if err != nil {
		t.Fatal(err)
	}
	// the subtree's child is a concat; drive it with an empty skel list so the
	// "no skel for this child" (nil) branch runs.
	node := forest[0]
	concat := l.child // subtree child
	var out strings.Builder
	s := &putState{out: &out, tree: node,
		split: &psplit{nodes: node.Children, enc: encodeForest(node.Children)},
		skel:  &skel{tag: lConcat}}
	safe(func() { putConcat(concat, s) })
}

func TestCreateUnionNoBranch(t *testing.T) {
	l := compileLens(t, ` let lns = [ key /[a-z]/ . store /[0-9]/ ] | [ key /[A-Z]/ . store /[0-9]/ ]`)
	// a tree encoding that matches neither branch's atype
	var out strings.Builder
	s := &putState{out: &out, split: &psplit{nodes: nil, enc: "9\x039\x04"}}
	createUnion(l, s)
	if s.err == nil {
		t.Error("createUnion with no matching branch should fail")
	}
}

func TestCreateSplitErrors(t *testing.T) {
	defer clearInject()
	boom := errors.New("boom")
	concat := compileLens(t, ` let lns = [ key /[a-z]+/ . store /[0-9]+/ ] . [ key /[a-z]+/ . store /[0-9]+/ ]`)
	star := compileLens(t, ` let lns = [ key /[a-z]+/ . store /[0-9]+/ ]*`)
	sq := compileLens(t, ` let lns = [ square (key /[a-z]+/) (store /[0-9]+/) (del /[a-z]+/ "z") ]`)
	forest := []*Tree{{Label: strptr("a"), Value: strptr("1")}}
	drive := func(l *Lens, f []*Tree) {
		var out strings.Builder
		s := &putState{out: &out, split: &psplit{nodes: f, enc: encodeForest(f)}}
		injectAt(1, nil, false, boom)
		safe(func() { createLens(l, s) })
	}
	for k := 1; k <= 4; k++ {
		injectAt(k, nil, false, boom)
		drive(concat, []*Tree{{Label: strptr("a"), Value: strptr("1")}, {Label: strptr("b"), Value: strptr("2")}})
		injectAt(k, nil, false, boom)
		drive(star, forest)
		injectAt(k, nil, false, boom)
		drive(sq, []*Tree{{Label: strptr("ab"), Value: strptr("5")}})
	}
}

func TestSplitConcatUnmatchedGroup(t *testing.T) {
	defer clearInject()
	l := compileLens(t, ` let lns = [ key /[a-z]+/ . store /[0-9]+/ ] . [ key /[a-z]+/ . store /[0-9]+/ ]`)
	forest := []*Tree{{Label: strptr("a"), Value: strptr("1")}, {Label: strptr("b"), Value: strptr("2")}}
	enc := encodeForest(forest) // 8 bytes
	// inject a full match whose first child group is unmatched (start -1)
	injectAt(1, []int{0, len(enc), -1, -1}, true, nil)
	if _, err := splitConcat(&psplit{nodes: forest, enc: enc}, l); err == nil {
		t.Error("splitConcat with unmatched group should error")
	}
}

func TestPutSquareShortSkel(t *testing.T) {
	l := compileLens(t, ` let lns = [ square (key /[a-z]+/) (store /[0-9]+/) (del /[a-z]+/ "z") ]`)
	forest, err := LnsGet(l, "ab5ab")
	if err != nil {
		t.Fatal(err)
	}
	node := forest[0]
	var out strings.Builder
	// square with a nil skel -> per-child skel selection takes the else branch
	s := &putState{out: &out, tree: node,
		split: &psplit{nodes: node.Children, enc: encodeForest(node.Children)}, skel: nil}
	safe(func() { putSquare(l.child, s) })
}

func TestRecursiveSquareNocase(t *testing.T) {
	// recursive lens containing a nocase square
	src := "module M =\n let rec lns = [ square (key /[a-z]+/i) (store /[0-9]/ . lns?) (del /[a-z]+/i \"z\") ] | [ label \"x\" . store /!/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	if l, err := i.LensValue("M", "lns"); err == nil {
		safe(func() { LnsGet(l, "AB5ab") })
		safe(func() { LnsGet(l, "AB5!ab") })
	}
}

func TestRecParseConcatDedup(t *testing.T) {
	// Directly exercise the acc dedup: child0 is length-ambiguous (ends 1 and 2)
	// and child1 converges both endpoints to end 3, so the second path to end 3
	// is dropped by the seen-check.
	c0 := &Lens{tag: lConcat}
	c1 := &Lens{tag: lConcat}
	s := &recState{text: "abc", memo: map[recKey][]recResult{}, busy: map[recKey]bool{}, seqs: map[string]int{}}
	s.memo[recKey{c0, 0}] = []recResult{{end: 1}, {end: 2}}
	s.memo[recKey{c1, 1}] = []recResult{{end: 3}}
	s.memo[recKey{c1, 2}] = []recResult{{end: 3}}
	if out := s.parseRecConcat([]*Lens{c0, c1}, 0); len(out) != 1 || out[0].end != 3 {
		t.Fatalf("dedup: %d results", len(out))
	}
}

func TestParseRecTopPartial(t *testing.T) {
	// recursive parse that cannot consume the whole input -> "did not match".
	src := "module M =\n let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ] | [ label \"x\" . store /[a-z]/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lnsParse(l, "(a"); err == nil {
		t.Error("recursive parse of incomplete input should fail")
	}
}

func TestGetRecErr(t *testing.T) {
	defer clearInject()
	// injected match error during a recursive get sets s.err, hitting getRec's
	// error return.
	src := "module M =\n let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ] | [ label \"x\" . store /[a-z]/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatal(err)
	}
	for k := 1; k <= 10; k++ {
		injectAt(k, nil, false, errors.New("boom"))
		safe(func() { LnsGet(l, "(a)") })
	}
}

func TestParseRecConcatDedupDirect(t *testing.T) {
	// Two child lenses where the first is length-ambiguous over "ab" (matching
	// "a" or "ab") and the second converges both endpoints to the same end,
	// forcing the acc dedup in parseRecConcat.
	src := "module M =\n" +
		" let c0 = [ label \"a\" . store /a/ ] | [ label \"a\" . store /ab/ ]\n" +
		" let c1 = [ del /b/ \"b\" ] | [ label \"e\" ]\n" +
		" let lns = c0 . c1\n"
	i := New(srcMap(map[string]string{"m": src}))
	c0, err := i.LensValue("M", "c0")
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := i.LensValue("M", "c1")
	s := &recState{text: "ab", memo: map[recKey][]recResult{}, busy: map[recKey]bool{}, seqs: map[string]int{}}
	_ = s.parseRecConcat([]*Lens{c0, c1}, 0)
}

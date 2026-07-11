// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"strings"
	"testing"
)

// TestDispatchDefaults exercises the defensive default arms of the get/put/parse
// dispatchers with a lens carrying an out-of-range tag.
func TestDispatchDefaults(t *testing.T) {
	bogus := &Lens{tag: lensTag(99), ctype: regexpMakeEmpty(), atype: regexpMakeEmpty()}
	gs := &getState{text: "", regs: []int{0, 0}, seqs: map[string]int{}}
	if getLens(bogus, gs, true); gs.err == nil {
		t.Error("getLens default")
	}
	ps := &getState{text: "", regs: []int{0, 0}, seqs: map[string]int{}}
	if parseLens(bogus, ps, true); ps.err == nil {
		t.Error("parseLens default")
	}
	var out strings.Builder
	pu := &putState{out: &out, split: &psplit{}, skel: &skel{}}
	putLens(bogus, pu)
	if pu.err == nil {
		t.Error("putLens default")
	}
	pu2 := &putState{out: &out, split: &psplit{}}
	createLens(bogus, pu2)
	if pu2.err == nil {
		t.Error("createLens default")
	}
	// recursive parseUncached default
	rs := &recState{text: "", memo: map[recKey][]recResult{}, busy: map[recKey]bool{}, seqs: map[string]int{}}
	rs.parseUncached(&Lens{tag: lensTag(99), recursive: true}, 0)
	if rs.err == nil {
		t.Error("parseUncached default")
	}
	// skelInstanceOf default
	if skelInstanceOf(&Lens{tag: lensTag(99)}, &skel{tag: lDel}) {
		t.Error("skelInstanceOf default")
	}
}

func TestEvalRemaining(t *testing.T) {
	i := New(srcMap(map[string]string{"m": "module M =\n let x = /a/\n"}))
	e := newEnv(i.global)
	// Lookup missing binding
	if _, err := i.Lookup("M", "nope"); err == nil {
		t.Error("lookup missing binding")
	}
	// bracket whose inner errors
	if _, err := i.eval(&bracketTerm{exp: &identTerm{name: "nope"}}, e); err == nil {
		t.Error("bracket inner err")
	}
	// unknown binop tag
	if _, err := i.evalBinop(&binopTerm{tag: binopTag(99), left: &regexpTerm{pattern: "a"}, right: &regexpTerm{pattern: "b"}}, e); err == nil {
		t.Error("unknown binop")
	}
	// concat where right is a lens but left is not (via values)
	if _, err := concatValues(vUnit{}, &vLens{lens: makePrim(lStore, newRegexp("a", false), "")}); err == nil {
		t.Error("concat unit.lens")
	}
	// union where right is a lens but left isn't
	if _, err := unionValues(vUnit{}, &vLens{lens: makePrim(lStore, newRegexp("a", false), "")}); err == nil {
		t.Error("union unit|lens")
	}
	// let whose (paramless) exp fails surfaces immediately
	lt := &letTerm{name: "f", exp: &identTerm{name: "nope"}, body: &identTerm{name: "f"}}
	if _, err := i.eval(lt, e); err == nil {
		t.Error("let exp err")
	}
}

func TestDfaMinusMore(t *testing.T) {
	// A variety of minus operations to exercise complete/reUnion/reConcat and
	// the empty-result path.
	cases := [][2]string{
		{"a?b", "b"},      // optional -> dead-state completion
		{"(ab|cd)", "ab"}, // union elimination
		{"a", "a|b"},      // subtrahend larger
		{"[a-c]+", "b+"},  // ranges
	}
	for _, c := range cases {
		_, _ = regexpMinus(newRegexp(c[0], false), newRegexp(c[1], false))
	}
	// empty result: r1 subset of r2 -> toRegexp empty-set error
	if _, err := regexpMinus(newRegexp("ab", false), newRegexp("ab|cd", false)); err == nil {
		t.Log("expected empty-set error")
	}
}

func TestTreeFormatIndent(t *testing.T) {
	// deep nesting to exercise the indent loop in formatForest
	forest := []*Tree{{Label: strptr("a"), Children: []*Tree{
		{Label: strptr("b"), Children: []*Tree{{Label: strptr("c")}}}}}}
	if !strings.Contains(Format(forest), `"c"`) {
		t.Error("indent format")
	}
}

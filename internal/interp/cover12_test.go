// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"errors"
	"testing"
)

func TestRecursivePutCreateFromScratch(t *testing.T) {
	src := "module M =\n let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ] | [ label \"x\" . store /[a-z]/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatal(err)
	}
	// parse an empty group, then add nested nodes: put must create them.
	forest, err := LnsGet(l, "()")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	root := &Tree{Children: forest}
	if err := treeCmdSet(root, "/g/x", "a"); err != nil {
		t.Fatal(err)
	}
	out, err := LnsPut(l, root.Children, "()")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if out != "(a)" {
		t.Fatalf("create put: %q", out)
	}
	// put a tree that matches no union branch -> create/put union error
	safe(func() { LnsPut(l, []*Tree{{Label: strptr("bogus"), Value: strptr("Z")}}, "()") })
	safe(func() { LnsPut(l, []*Tree{{Label: strptr("g"), Children: []*Tree{{Label: strptr("bogus")}}}}, "()") })
}

func TestRecursiveNoProgressAndDedup(t *testing.T) {
	// star child that can match empty text (label branch) under recursion.
	src := "module M =\n let rec lns = ([ label \"e\" ] | [ del /a/ \"a\" . lns ])\n"
	i := New(srcMap(map[string]string{"m": src}))
	if l, err := i.LensValue("M", "lns"); err == nil {
		safe(func() { LnsGet(l, "") })
		safe(func() { LnsGet(l, "a") })
	}
	// ambiguous recursive alternatives reaching the same end (dedup path)
	src2 := "module N =\n let rec lns = [ key /a/ . store /b/ ] | [ key /a/ . store /b/ ] | [ del /c/ \"c\" . lns ]\n"
	i2 := New(srcMap(map[string]string{"n": src2}))
	if l, err := i2.LensValue("N", "lns"); err == nil {
		safe(func() { LnsGet(l, "ab") })
		safe(func() { LnsGet(l, "cab") })
	}
}

func TestGetFaultGuards(t *testing.T) {
	defer clearInject()
	boom := errors.New("boom")
	// concat: star child errors -> following child's getLens s.err guard
	l := compileLens(t, ` let lns = del /a/ "a" . (del /b/ "b")* . del /c/ "c"`)
	for k := 1; k <= 6; k++ {
		injectAt(k, nil, false, boom)
		safe(func() { LnsGet(l, "abbc") })
	}
	// recursive terminal whose internal star match errors -> parseTerminal gs.err
	src := "module M =\n let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ] | [ label \"x\" . (del /b/ \"b\")* . store /[a-z]/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	if rl, err := i.LensValue("M", "lns"); err == nil {
		for k := 1; k <= 12; k++ {
			injectAt(k, nil, false, boom)
			safe(func() { LnsGet(rl, "(bba)") })
		}
	}
}

func TestDfaDedupUnion(t *testing.T) {
	// Symmetric automata where an existing edge equals an eliminated path,
	// exercising reUnion's identical-operand shortcut.
	for _, p := range [][2]string{
		{"(a|a)*", "b"},
		{"aa|aa", "b"},
		{"(ab|ab)c", "z"},
	} {
		_, _ = regexpMinus(newRegexp(p[0], false), newRegexp(p[1], false))
	}
}

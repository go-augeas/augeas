// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"errors"
	"testing"
)

func TestPutFaultInjectionRecursive(t *testing.T) {
	defer clearInject()
	boom := errors.New("boom")
	src := "module M =\n let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ] | [ label \"x\" . store /[a-z]/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatal(err)
	}
	// Base tree (reuse), and a tree with additions (create).
	base, _ := LnsGet(l, "(a)")
	root := &Tree{Children: base}
	safe(func() { treeCmdSet(root, "/g/x[last()+1]", "b") })
	trees := [][]*Tree{base, root.Children,
		{{Label: strptr("g"), Children: []*Tree{{Label: strptr("x"), Value: strptr("z")}, {Label: strptr("x"), Value: strptr("y")}}}}}
	for _, f := range trees {
		for k := 1; k <= 20; k++ {
			injectAt(k, nil, false, boom)
			safe(func() { LnsPut(l, f, "(a)") })
		}
	}
}

func TestRecursiveStarEmptyChild(t *testing.T) {
	// A genuinely recursive lens with a star whose child can match empty text.
	src := "module M =\n let rec lns = ([ label \"e\" ] | [ del /a/ \"a\" . lns ])*\n"
	i := New(srcMap(map[string]string{"m": src}))
	if l, err := i.LensValue("M", "lns"); err == nil {
		safe(func() { LnsGet(l, "") })
		safe(func() { LnsGet(l, "aa") })
	}
}

func TestPutSubtreeNoNode(t *testing.T) {
	// A subtree lens applied to an empty forest via fault-free put where the
	// split yields no node exercises the guard.
	l := compileLens(t, ` let lns = [ label "x" . store /[0-9]+/ ]*`)
	// Empty forest -> star with zero elements; then a hand-built inconsistent
	// tree drives the subtree with an empty split.
	safe(func() { LnsPut(l, nil, "") })
	safe(func() { LnsPut(l, []*Tree{}, "5") })
}

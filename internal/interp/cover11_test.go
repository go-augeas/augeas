// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func TestFinalBranches(t *testing.T) {
	// concatValues: left not a lens, right is a lens
	if _, err := concatValues(vUnit{}, &vLens{lens: makePrim(lStore, newRegexp("a", false), "")}); err == nil {
		t.Error("concat unit.lens")
	}

	// parser: compose right operand error, tree child error
	for _, s := range []string{
		"module M =\n let x = del /a/ \"a\" ; )\n",
		"module M =\n let x = { \"a\" { ) }\n",
	} {
		if _, err := parseModule(s); err == nil {
			t.Errorf("expected parse error %q", s)
		}
	}

	// get native whose LnsGet fails
	i := New(srcMap(map[string]string{}))
	getFn, _ := i.global.lookup("get")
	del := &vLens{lens: makePrim(lDel, newRegexp("[0-9]+", false), "0")}
	v, _ := i.apply(getFn, del)
	if _, err := i.apply(v, vString("zzz")); err == nil {
		t.Error("get native should fail on non-matching input")
	}

	// nested comments in the lexer
	if _, err := parseModule("module M =\n(* a (* b *) c *)\n let x = /a/\n"); err != nil {
		t.Errorf("nested comment: %v", err)
	}

	// restrictRE2 trailing backslash (outside a class) and POSIX positive class
	_ = restrictRE2(`a\`)
	_ = restrictRE2(`[[:digit:]]`)

	// findChildren predLast with no match
	r := &Tree{Children: []*Tree{{Label: strptr("a")}}}
	segs, _ := parsePath("/x[last()]")
	if len(findNodes(r, segs)) != 0 {
		t.Error("predLast empty")
	}

	// createPathTree: an intermediate segment matching >1 node -> error
	r2 := &Tree{Children: []*Tree{{Label: strptr("a")}, {Label: strptr("a")}}}
	if err := treeCmdSet(r2, "/a/z", "v"); err == nil {
		t.Error("set through ambiguous intermediate")
	}
	if err := treeCmdClear(r2, "/a/z"); err == nil {
		t.Error("clear through ambiguous intermediate")
	}

	// insa non-string path; insb non-string label (natives, all args applied)
	tr := &vTree{forest: []*Tree{{Label: strptr("a")}}}
	apply3 := func(name string, a, b, c Value) error {
		fn, _ := i.global.lookup(name)
		v, err := i.apply(fn, a)
		if err != nil {
			return err
		}
		if v, err = i.apply(v, b); err != nil {
			return err
		}
		_, err = i.apply(v, c)
		return err
	}
	if apply3("insa", vString("z"), vUnit{}, tr) == nil {
		t.Error("insa non-string path")
	}
	if apply3("insb", vUnit{}, vString("/a"), tr) == nil {
		t.Error("insb non-string label")
	}
}

func TestFinalBranches2(t *testing.T) {
	// concatValues: left IS a lens, right is not
	if _, err := concatValues(&vLens{lens: makePrim(lStore, newRegexp("a", false), "")}, vUnit{}); err == nil {
		t.Error("concat lens.unit")
	}
	// deeply-nested tree with missing closing braces -> parseTreeBranch error
	if _, err := parseModule("module M =\n let x = { \"a\" { \"b\" { \"c\"\n"); err == nil {
		t.Error("nested tree missing braces")
	}
}

func TestRecursivePutCreate(t *testing.T) {
	// Recursive lens; add a nested node so put must create fresh subtrees
	// (exercises the create_* paths for recursive lenses).
	src := "module M =\n let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ] | [ label \"x\" . store /[a-z]/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	in := "(a)"
	forest, err := LnsGet(l, in)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// add a sibling 'x' node inside the group, and modify
	root := &Tree{Children: forest}
	safe(func() { treeCmdSet(root, "/g/x[last()+1]", "b") })
	safe(func() { LnsPut(l, root.Children, in) })
	// a union recursive lens where the tree matches neither branch cleanly
	safe(func() { LnsPut(l, []*Tree{{Label: strptr("bogus"), Value: strptr("Z")}}, in) })
}

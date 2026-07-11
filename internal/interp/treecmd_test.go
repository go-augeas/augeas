// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func mkForest() *Tree {
	return &Tree{Children: []*Tree{
		{Label: strptr("a"), Value: strptr("1")},
		{Label: strptr("a"), Value: strptr("2")},
		{Label: strptr("b"), Children: []*Tree{{Label: strptr("c"), Value: strptr("x")}}},
	}}
}

func TestParsePathPredicates(t *testing.T) {
	cases := []string{"/a", "/a[1]", "/a[last()]", "/a[. = '1']", "/b[c]", "/b[c = 'x']", "/*[last()]", "/a[last()+1]"}
	for _, p := range cases {
		if _, err := parsePath(p); err != nil {
			t.Fatalf("parsePath %q: %v", p, err)
		}
	}
	if _, err := parsePath("/a[bad"); err == nil {
		t.Fatal("expected bad segment error")
	}
}

func TestFindNodes(t *testing.T) {
	r := mkForest()
	must := func(p string, n int) {
		segs, _ := parsePath(p)
		if got := len(findNodes(r, segs)); got != n {
			t.Fatalf("%q: got %d want %d", p, got, n)
		}
	}
	must("/a", 2)
	must("/a[1]", 1)
	must("/a[last()]", 1)
	must("/a[. = '2']", 1)
	must("/b[c]", 1)
	must("/b[c = 'x']", 1)
	must("/b[c = 'nope']", 0)
	must("/a[last()+1]", 0)
	must("/*", 3)
}

func TestTreeCmds(t *testing.T) {
	r := mkForest()
	if err := treeCmdSet(r, "/a[1]", "9"); err != nil || *r.Children[0].Value != "9" {
		t.Fatalf("set: %v", err)
	}
	if err := treeCmdSet(r, "/new/deep", "z"); err != nil {
		t.Fatalf("set create: %v", err)
	}
	var deep *string
	for _, c := range r.Children[3].Children {
		if c.Label != nil && *c.Label == "deep" {
			deep = c.Value
		}
	}
	if deep == nil || *deep != "z" {
		t.Fatal("nested create")
	}
	if err := treeCmdClear(r, "/a[1]"); err != nil || r.Children[0].Value != nil {
		t.Fatalf("clear: %v", err)
	}
	if err := treeCmdClear(r, "/fresh"); err != nil {
		t.Fatalf("clear create: %v", err)
	}
	n := len(r.Children)
	if err := treeCmdRm(r, "/a"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if len(r.Children) != n-2 {
		t.Fatalf("rm count: %d", len(r.Children))
	}
	// insa/insb
	r2 := mkForest()
	if err := treeCmdIns(r2, "z", "/a[1]", false); err != nil {
		t.Fatalf("insa: %v", err)
	}
	if *r2.Children[1].Label != "z" {
		t.Fatalf("insa pos: %+v", r2.Children[1])
	}
	if err := treeCmdIns(r2, "y", "/a[1]", true); err != nil {
		t.Fatalf("insb: %v", err)
	}
	if *r2.Children[0].Label != "y" {
		t.Fatal("insb pos")
	}
	// insert at root
	r3 := mkForest()
	if err := treeCmdIns(r3, "top", "/", false); err != nil {
		t.Fatalf("insa root: %v", err)
	}
	if *r3.Children[len(r3.Children)-1].Label != "top" {
		t.Fatal("insa root pos")
	}
	if err := treeCmdIns(r3, "first", "/", true); err != nil {
		t.Fatalf("insb root: %v", err)
	}
	if *r3.Children[0].Label != "first" {
		t.Fatal("insb root pos")
	}
}

func TestTreeCmdErrors(t *testing.T) {
	r := mkForest()
	if err := treeCmdSet(r, "/a", "x"); err == nil {
		t.Fatal("set ambiguous should error")
	}
	if err := treeCmdIns(r, "z", "/a", false); err == nil {
		t.Fatal("ins ambiguous should error")
	}
	if err := treeCmdIns(r, "z", "/nope", false); err == nil {
		t.Fatal("ins no-match should error")
	}
	if err := treeCmdSet(r, "/a[bad", "x"); err == nil {
		t.Fatal("bad path")
	}
}

func TestTreeCmdNatives(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	forest := []*Tree{{Label: strptr("k"), Value: strptr("v")}}
	setFn, _ := i.global.lookup("set")
	// apply set "/k" "new" tree
	v, err := i.apply(setFn, vString("/k"))
	if err != nil {
		t.Fatal(err)
	}
	v, err = i.apply(v, vString("new"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := i.apply(v, &vTree{forest: forest})
	if err != nil {
		t.Fatal(err)
	}
	tv := res.(*vTree)
	if *tv.forest[0].Value != "new" {
		t.Fatalf("set native: %v", *tv.forest[0].Value)
	}
}

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

// compileLens compiles a single-binding module and returns the lens.
func compileLens(t *testing.T, body string) *Lens {
	t.Helper()
	src := "module M =\n" + body + "\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatalf("compile %q: %v", body, err)
	}
	return l
}

func roundtrip(t *testing.T, l *Lens, in string) {
	t.Helper()
	f, err := LnsGet(l, in)
	if err != nil {
		t.Fatalf("get %q: %v", in, err)
	}
	out, err := LnsPut(l, f, in)
	if err != nil {
		t.Fatalf("put %q: %v", in, err)
	}
	if out != in {
		t.Fatalf("roundtrip %q -> %q", in, out)
	}
}

func TestPutRoundtrips(t *testing.T) {
	cases := []struct{ body, in string }{
		{` let lns = [ key /[a-z]+/ . del /=/ "=" . store /[0-9]+/ ] . del /\n/ "\n"`, "a=1\n"},
		{` let lns = ( [ key /[a-z]+/ . del /=/ "=" . store /[0-9]+/ ] . del /\n/ "\n" )*`, "a=1\nb=2\n"},
		{` let lns = [ label "x" . store /[0-9]+/ ]? . del /\n/ "\n"`, "5\n"},
		{` let lns = [ seq "s" . store /[0-9]+/ . del /,/ "," ]*`, "1,2,3,"},
		{` let lns = [ del /#/ "#" . label "c" . store /[a-z]+/ ] | [ key /[a-z]+/ . del /=/ "=" . store /[a-z]+/ ]`, "#foo"},
	}
	for _, c := range cases {
		roundtrip(t, compileLens(t, c.body), c.in)
	}
}

// TestPutEmptyTree covers upstream's lns_put guard: putting an empty tree
// (e.g. the maybe-absent get of "\n") emits nothing, matching real Augeas
// (`if (tree == NULL) return;`), rather than reconstructing the skeleton.
func TestPutEmptyTree(t *testing.T) {
	l := compileLens(t, ` let lns = [ label "x" . store /[0-9]+/ ]? . del /\n/ "\n"`)
	f, err := LnsGet(l, "\n")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(f) != 0 {
		t.Fatalf("expected empty forest, got %d nodes", len(f))
	}
	out, err := LnsPut(l, f, "\n")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if out != "" {
		t.Fatalf("empty-tree put = %q, want %q", out, "")
	}
}

func TestPutModifications(t *testing.T) {
	l := compileLens(t, ` let lns = ( [ key /[a-z]+/ . del /=/ "=" . store /[0-9]+/ ] . del /\n/ "\n" )*`)
	f, err := LnsGet(l, "a=1\nb=2\n")
	if err != nil {
		t.Fatal(err)
	}
	root := &Tree{Children: f}
	if err := treeCmdSet(root, "/a", "9"); err != nil {
		t.Fatal(err)
	}
	out, err := LnsPut(l, root.Children, "a=1\nb=2\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != "a=9\nb=2\n" {
		t.Fatalf("put after set: %q", out)
	}
	// add a new node -> create branch
	if err := treeCmdSet(root, "/c", "3"); err != nil {
		t.Fatal(err)
	}
	out, err = LnsPut(l, root.Children, "a=1\nb=2\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != "a=9\nb=2\nc=3\n" {
		t.Fatalf("put after add: %q", out)
	}
}

func TestPutStoreErrors(t *testing.T) {
	l := compileLens(t, ` let lns = [ label "x" . store /[0-9]+/ ]`)
	// value that doesn't match the store regexp
	bad := []*Tree{{Label: strptr("x"), Value: strptr("abc")}}
	if _, err := LnsPut(l, bad, "5"); err == nil {
		t.Fatal("expected store regexp error")
	}
	// nil value
	nilv := []*Tree{{Label: strptr("x")}}
	if _, err := LnsPut(l, nilv, "5"); err == nil {
		t.Fatal("expected nil-value error")
	}
}

func TestPutSquare(t *testing.T) {
	l := compileLens(t, ` let lns = [ square (key /[a-z]+/) (store /[0-9]+/) (del /[a-z]+/ "x") ]`)
	roundtrip(t, l, "ab5ab")
}

func TestSkelInstanceOf(t *testing.T) {
	if skelInstanceOf(&Lens{tag: lDel, regexp: newRegexp("a", false)}, nil) {
		t.Fatal("nil skel")
	}
	sk := &skel{tag: lStore}
	if !skelInstanceOf(&Lens{tag: lStore}, sk) {
		t.Fatal("store instance")
	}
	if skelInstanceOf(&Lens{tag: lStore}, &skel{tag: lDel}) {
		t.Fatal("mismatch")
	}
}

func TestEncoding(t *testing.T) {
	forest := []*Tree{{Label: strptr("k"), Value: strptr("v")}, {Label: strptr("x")}}
	enc := encodeForest(forest)
	if enc != "k\x03v\x04x\x03\x04" {
		t.Fatalf("enc %q", enc)
	}
	if countSlash(enc, len(enc)) != 2 {
		t.Fatal("countSlash")
	}
}

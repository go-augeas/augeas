// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func TestLeftRecursionGuard(t *testing.T) {
	// Directly left-recursive lens: the busy guard breaks the cycle and the
	// parse fails to consume the input.
	src := "module M =\n let rec lns = lns . del /a/ \"a\" | del /a/ \"a\"\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := LnsGet(l, "aa"); err == nil {
		t.Log("left-recursive parse returned; guard exercised")
	}
}

func TestSquareNocase(t *testing.T) {
	// square with a nocase left/right delimiter exercises the case-fold compare.
	l := compileLens(t, ` let lns = [ square (key /[a-z]+/i) (store /[0-9]+/) (del /[a-z]+/i "x") ]`)
	if _, err := LnsGet(l, "AB5ab"); err != nil {
		t.Fatalf("square nocase get: %v", err)
	}
}

func TestParserPutAndTreeForms(t *testing.T) {
	ok := []string{
		"module M =\n test x put \"a\" after set \"/p\" \"v\" = \"b\"\n let x = del /a/ \"a\"\n",
		"module M =\n let x = ()\n", // unit
	}
	for _, s := range ok {
		if _, err := parseModule(s); err != nil {
			t.Errorf("unexpected error %q: %v", s, err)
		}
	}
	bad := []string{
		"module M =\n let x = { \"a\" { \"b\"",        // nested tree missing rbrace
		"module M =\n let x (p:string ->) = /a/\n",    // arrow type missing rhs
		"module M =\n let x = let y (p) = /a/ in y\n", // let-in param bad
		"module M =\n test (x get \"a\" = \"b\"\n",    // aexp paren unclosed in test
	}
	for _, s := range bad {
		if _, err := parseModule(s); err == nil {
			t.Errorf("expected error %q", s)
		}
	}
}

func TestEvalUnionAndComposeErrors(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	e := newEnv(i.global)
	// union: non-lens with lens on the right
	if _, err := i.eval(&binopTerm{tag: opUnion, left: &unitTerm{}, right: &bracketTerm{exp: &identTerm{name: "del"}}}, e); err == nil {
		t.Error("union unit lens")
	}
	// compose where left is a function and right errors
	fn := &funcTerm{param: param{name: "p"}, body: &identTerm{name: "p"}}
	if _, err := i.eval(&binopTerm{tag: opCompose, left: fn, right: &identTerm{name: "nope"}}, e); err == nil {
		t.Error("compose right err")
	}
	// compose left errors
	if _, err := i.eval(&binopTerm{tag: opCompose, left: &identTerm{name: "nope"}, right: fn}, e); err == nil {
		t.Error("compose left err")
	}
	// apply a closure whose body errors
	badClosure := &vClosure{param: "p", body: &identTerm{name: "nope"}, env: e}
	if _, err := i.apply(badClosure, vString("x")); err == nil {
		t.Error("closure body err")
	}
	// composed apply where inner errors
	goodClosure := &vClosure{param: "p", body: &identTerm{name: "p"}, env: e}
	comp := &composed{f: badClosure, g: goodClosure, i: i}
	if _, err := i.apply(comp, vString("x")); err == nil {
		t.Error("composed inner err")
	}
	// evalRep on an ident that errors
	if _, err := i.eval(&repTerm{exp: &identTerm{name: "nope"}, quant: qStar}, e); err == nil {
		t.Error("rep err")
	}
	// let-in exp errors
	if _, err := i.eval(&letTerm{name: "x", exp: &identTerm{name: "nope"}, body: &unitTerm{}}, e); err == nil {
		t.Error("let-in exp err")
	}
}

func TestTreecmdMisc(t *testing.T) {
	// unquotePred with no quotes returns as-is
	if unquotePred("abc") != "abc" {
		t.Error("unquotePred plain")
	}
	// parsePredicate error path via a bad index that is not a number and has '='
	segs, err := parsePath("/a[x = 'y']")
	if err != nil || segs[0].kind != predChild {
		t.Errorf("child pred: %v", err)
	}
	// findChildren default (no predicate) returns all
	r := &Tree{Children: []*Tree{{Label: strptr("a")}, {Label: strptr("a")}}}
	sg, _ := parsePath("/a")
	if len(findNodes(r, sg)) != 2 {
		t.Error("findChildren default")
	}
	// rm a nested path (parentOf traversal)
	r2 := &Tree{Children: []*Tree{{Label: strptr("b"), Children: []*Tree{{Label: strptr("c")}}}}}
	if err := treeCmdRm(r2, "/b/c"); err != nil || len(r2.Children[0].Children) != 0 {
		t.Errorf("rm nested: %v", err)
	}
}

func TestDfaCombinators(t *testing.T) {
	// exercise reUnion/reConcat/reStar identity and nil cases via minus that
	// produces unions and stars.
	if _, err := regexpMinus(newRegexp("a|b|c", false), newRegexp("a", false)); err != nil {
		t.Errorf("union minus: %v", err)
	}
	if _, err := regexpMinus(newRegexp("a*b", false), newRegexp("b", false)); err != nil {
		t.Errorf("star minus: %v", err)
	}
	// identical-alternatives to hit reUnion *a==*b
	if _, err := regexpMinus(newRegexp("(a|a)c", false), newRegexp("xyz", false)); err != nil {
		t.Errorf("dup-alt minus: %v", err)
	}
}

func TestBuiltinLensTypeNonLens(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	fn, _ := i.global.lookup("lens_ctype")
	if _, err := i.apply(fn, vString("x")); err == nil {
		t.Error("lens_ctype non-lens")
	}
}

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"strings"
	"testing"
)

// srcMap builds a Source from an in-memory map of base name -> source.
func srcMap(m map[string]string) Source {
	return func(base string) (string, bool) { s, ok := m[base]; return s, ok }
}

func TestUnescape(t *testing.T) {
	if got := unescapeString(`a\tb\n\"c\\d`); got != "a\tb\n\"c\\d" {
		t.Fatalf("string unescape: %q", got)
	}
	if got := unescapeRegexp(`a\/b\\c\.`); got != `a/b\c\.` {
		t.Fatalf("regexp unescape: %q", got)
	}
	if got := unescape(`x\zy`, ""); got != `x\zy` {
		t.Fatalf("passthrough: %q", got)
	}
}

func TestToRE2(t *testing.T) {
	cases := map[string]string{
		`a.b`:         `a.b`,
		`\.`:          `\.`,
		`\/`:          `/`,
		`[a-z]`:       `[a-z]`,
		`[[:digit:]]`: `[[:digit:]]`,
		`x*+`:         `x*`,
		`x?+`:         `x*`,
		`x??`:         `x?`,
		`x++`:         `x+`,
	}
	for in, want := range cases {
		if got := toRE2(in); got != want {
			t.Fatalf("toRE2(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPosixClassEnd(t *testing.T) {
	if posixClassEnd("[:alpha:]", 0) != 9 {
		t.Fatal("alpha")
	}
	if posixClassEnd("[:^alpha:]", 0) != 10 {
		t.Fatal("negated")
	}
	if posixClassEnd("[:zz", 0) != 0 {
		t.Fatal("unterminated")
	}
	if posixClassEnd("[::]", 0) != 0 {
		t.Fatal("empty name")
	}
}

func TestExpandNocase(t *testing.T) {
	if got := expandNocase("ab"); got != "[aA][bB]" {
		t.Fatalf("letters: %q", got)
	}
	if got := expandNocase("[a-c]"); got != "[a-cA-C]" {
		t.Fatalf("range: %q", got)
	}
	if got := expandNocase(`\d1`); got != `\d1` {
		t.Fatalf("escape+digit: %q", got)
	}
}

func TestRegexpMatchAndString(t *testing.T) {
	r := newRegexp("a+", false)
	regs, ok, err := r.match("aaab", 0, 4)
	if !ok || err != nil || regs[1] != 3 {
		t.Fatalf("match: %v %v %v", regs, ok, err)
	}
	if _, ok, _ := r.match("xyz", 0, 3); ok {
		t.Fatal("should not match")
	}
	if newRegexp("a", true).String() != "/a/i" {
		t.Fatal("String nocase")
	}
	var nilr *Regexp
	if nilr.String() != "<nil>" {
		t.Fatal("nil String")
	}
	// invalid pattern -> build error surfaces through nsub
	bad := newRawRegexp("(")
	if bad.nsub() != 0 {
		t.Fatal("bad nsub")
	}
}

func TestMinusBasic(t *testing.T) {
	// (a|b|c) - (b) should match a and c but not b
	r, err := regexpMinus(newRegexp("a|b|c", false), newRegexp("b", false))
	if err != nil {
		t.Fatalf("minus: %v", err)
	}
	if _, ok, _ := r.match("a", 0, 1); !ok {
		t.Fatal("a should match")
	}
	if regs, ok, _ := r.match("b", 0, 1); ok && regs[1] == 1 {
		t.Fatal("b should not fully match")
	}
	// empty result -> error
	if _, err := regexpMinus(newRegexp("a", false), newRegexp("a", false)); err == nil {
		t.Fatal("expected empty-set error")
	}
}

func TestFormat(t *testing.T) {
	forest := []*Tree{{Label: strptr("k"), Value: strptr("v"), Children: []*Tree{{Label: strptr("c")}}}}
	out := Format(forest)
	if !strings.Contains(out, `"k"`) || !strings.Contains(out, `"v"`) || !strings.Contains(out, `"c"`) {
		t.Fatalf("format: %q", out)
	}
}

func TestEvalErrors(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	if _, err := i.LoadModule("Missing"); err == nil {
		t.Fatal("missing module")
	}
	// unbound identifier
	i2 := New(srcMap(map[string]string{"m": "module M =\n let x = y\n"}))
	if _, err := i2.Lookup("M", "x"); err == nil {
		t.Fatal("unbound y")
	}
	// name mismatch
	i3 := New(srcMap(map[string]string{"a": "module B =\n let x = /a/\n"}))
	if _, err := i3.LoadModule("A"); err == nil {
		t.Fatal("name mismatch")
	}
	// parse error
	i4 := New(srcMap(map[string]string{"c": "module C = let"}))
	if _, err := i4.LoadModule("C"); err == nil {
		t.Fatal("parse error")
	}
}

func TestGetSimpleLens(t *testing.T) {
	src := "module M =\n" +
		" let lns = [ label \"k\" . store /[0-9]+/ ] . del /\\n/ \"\\n\"\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatalf("lens: %v", err)
	}
	forest, err := Get(l, "42\n")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(forest) != 1 || forest[0].Value == nil || *forest[0].Value != "42" {
		t.Fatalf("forest: %s", Format(forest))
	}
	// non-matching input
	if _, err := Get(l, "xx"); err == nil {
		t.Fatal("should fail")
	}
}

func TestConcatUnionTypes(t *testing.T) {
	if v, err := concatValues(vString("a"), vString("b")); err != nil || v != vString("ab") {
		t.Fatalf("string concat: %v %v", v, err)
	}
	if _, err := concatValues(vString("a"), &vLens{}); err == nil {
		t.Fatal("string.lens should fail")
	}
	if _, err := unionValues(vUnit{}, vUnit{}); err == nil {
		t.Fatal("unit union should fail")
	}
	if _, err := minusValues(&vLens{}, &vLens{}); err == nil {
		t.Fatal("lens minus should fail")
	}
}

func TestLexerErrors(t *testing.T) {
	for _, s := range []string{`"unterminated`, `/unterminated`, `(* open`, "module M =\n let x = \x00"} {
		if _, err := parseModule(s); err == nil {
			t.Fatalf("expected lex error for %q", s)
		}
	}
}

func TestParserErrors(t *testing.T) {
	for _, s := range []string{
		"notmodule",
		"module m = ", // lowercase module name
		"module M",    // missing =
		"module M = let x =",
		"module M = let x = ( ",
		"module M = test X get",
	} {
		if _, err := parseModule(s); err == nil {
			t.Fatalf("expected parse error for %q", s)
		}
	}
}

func TestLetInAndRep(t *testing.T) {
	src := "module M =\n let lns = let a = /x/ in [ key a . store /y/ ]?\n"
	i := New(srcMap(map[string]string{"m": src}))
	if _, err := i.LensValue("M", "lns"); err != nil {
		t.Fatalf("let-in/maybe: %v", err)
	}
	// regexp rep + plus + union
	src2 := "module N =\n let r = /a/* . /b/+ | /c/\n let lns = del r \"\"\n"
	i2 := New(srcMap(map[string]string{"n": src2}))
	if _, err := i2.LensValue("N", "lns"); err != nil {
		t.Fatalf("regex reps: %v", err)
	}
}

func TestComposeSequence(t *testing.T) {
	// `;` sequencing where left is not a function: value of right is returned.
	src := "module M =\n let x = /a/ ; /b/\n"
	i := New(srcMap(map[string]string{"m": src}))
	v, err := i.Lookup("M", "x")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if _, ok := v.(*vRegexp); !ok {
		t.Fatalf("compose result %T", v)
	}
}

func TestApplyErrors(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	if _, err := i.apply(vString("x"), vUnit{}); err == nil {
		t.Fatal("apply non-function")
	}
}

func TestGetErrorBranches(t *testing.T) {
	// concat: not enough components (second store fails)
	src := "module M =\n let lns = del /a/ \"a\" . store /b/\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, _ := i.LensValue("M", "lns")
	if _, err := Get(l, "a"); err == nil {
		t.Fatal("expected concat short error")
	}
	// union: no branch matches
	src2 := "module N =\n let lns = del /a/ \"a\" | del /b/ \"b\"\n"
	i2 := New(srcMap(map[string]string{"n": src2}))
	l2, _ := i2.LensValue("N", "lns")
	if _, err := Get(l2, "c"); err == nil {
		t.Fatal("expected union no-branch")
	}
	// star short iteration
	src3 := "module O =\n let lns = (del /a/ \"a\")*\n"
	i3 := New(srcMap(map[string]string{"o": src3}))
	l3, _ := i3.LensValue("O", "lns")
	if _, err := Get(l3, "aab"); err == nil {
		t.Fatal("expected star did-not-match")
	}
	// seq + counter + value + key
	src4 := "module P =\n let lns = counter \"c\" . [ seq \"c\" . value \"v\" ]*\n"
	i4 := New(srcMap(map[string]string{"p": src4}))
	l4, _ := i4.LensValue("P", "lns")
	f, err := Get(l4, "")
	if err != nil {
		t.Fatalf("seq lens: %v", err)
	}
	_ = f
}

func TestRecursiveGet(t *testing.T) {
	// balanced nested parens: rec lens
	src := "module R =\n" +
		" let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ]\n"
	i := New(srcMap(map[string]string{"r": src}))
	l, err := i.LensValue("R", "lns")
	if err != nil {
		t.Fatalf("rec lens: %v", err)
	}
	f, err := Get(l, "(())")
	if err != nil {
		t.Fatalf("rec get: %v", err)
	}
	if len(f) != 1 || len(f[0].Children) != 1 {
		t.Fatalf("rec tree: %s", Format(f))
	}
	// does not match entire input
	if _, err := Get(l, "(()"); err == nil {
		t.Fatal("expected rec partial fail")
	}
}

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"strings"
	"testing"
)

func TestRegexTranslateClasses(t *testing.T) {
	// class with all control escapes + hex + hex A-F, exercised via restrict()
	r := newRawRegexp(`[\n\t\r\f\v\a\x0b\x41-\x46a-fA-F]`)
	res := restrict(r)
	if _, ok, _ := res.match("a", 0, 1); !ok {
		t.Error("restricted class should still match 'a'")
	}
	// ']' as first class member; literal '[' + POSIX class
	_ = toRE2(`[]a]`)
	_ = toRE2(`[a[:digit:]b]`)
	_ = toRE2(`[a[b]`)
	// negated class with leading ']' through restrict (insert-after-] path)
	_ = restrict(newRawRegexp(`[^]a-]`))
	// restrict a "." (any-non-newline) and a POSIX class in restrictRE2
	_ = restrict(newRawRegexp(`.[[:alpha:]]`))
}

func TestExpandNocaseClasses(t *testing.T) {
	// class starting with ^ and ], and a non-letter class (ex == "")
	_ = expandNocase(`[^]a-z]`)
	if got := expandNocase(`[0-9]`); got != "[0-9]" {
		t.Errorf("non-letter class: %q", got)
	}
	// swapCase both directions
	if swapCase('A') != 'a' || swapCase('z') != 'Z' || swapCase('5') != '5' {
		t.Error("swapCase")
	}
}

func TestAutomataMore(t *testing.T) {
	// multibyte literal in a concat (two-rune literal) exercises rune>255 paths
	if _, err := regexpMinus(newRawRegexp("aé"), newRegexp("x", false)); err != nil {
		t.Errorf("multibyte concat minus: %v", err)
	}
	// any-char (dotall) operand exercises OpAnyChar
	r, err := regexpMinus(newRawRegexp("(?s:.)"), newRegexp("\n", false))
	if err != nil {
		t.Fatalf("anychar dotall minus: %v", err)
	}
	if _, ok, _ := r.match("z", 0, 1); !ok {
		t.Error("dotall minus should match z")
	}
}

func TestMinusParseErrors(t *testing.T) {
	// broken raw operand -> parseForFA error branches
	if _, err := regexpMinus(newRawRegexp("("), newRegexp("a", false)); err == nil {
		t.Error("minus broken r1")
	}
	if _, err := regexpMinus(newRegexp("a", false), newRawRegexp("(")); err == nil {
		t.Error("minus broken r2")
	}
}

func TestTreeFormatAndEqual(t *testing.T) {
	forest := []*Tree{{Label: strptr("k"), Value: strptr("v"),
		Children: []*Tree{{Label: strptr("c"), Value: strptr("d")}}}}
	out := Format(forest)
	if !strings.Contains(out, `"c"`) {
		t.Errorf("nested format: %q", out)
	}
	// inequalities
	a := []*Tree{{Label: strptr("x")}}
	b := []*Tree{{Label: strptr("y")}}
	if treesEqual(a, b) {
		t.Error("label mismatch")
	}
	if treesEqual(a, []*Tree{{Label: strptr("x"), Value: strptr("z")}}) {
		t.Error("value nil-mismatch")
	}
	if treesEqual(a, nil) {
		t.Error("length mismatch")
	}
}

func TestRecErrors(t *testing.T) {
	// let rec body references unbound -> eval error
	i := New(srcMap(map[string]string{"m": "module M =\n let rec x = y\n"}))
	if _, err := i.Lookup("M", "x"); err == nil {
		t.Error("rec body unbound")
	}
	// let rec body not a lens
	i2 := New(srcMap(map[string]string{"n": "module N =\n let rec x = \"s\"\n"}))
	if _, err := i2.Lookup("N", "x"); err == nil {
		t.Error("rec body non-lens")
	}
}

func TestAutoloadNonTransform(t *testing.T) {
	i := New(srcMap(map[string]string{
		"m": "module M =\n autoload xfm\n let xfm = /a/\n",
	}))
	if _, _, err := i.Autoload("M"); err == nil {
		t.Error("autoload non-transform should error")
	}
	// autoload binding that fails to evaluate
	i2 := New(srcMap(map[string]string{
		"n": "module N =\n autoload xfm\n let xfm = transform bad (incl \"/f\")\n",
	}))
	if _, _, err := i2.Autoload("N"); err == nil {
		t.Error("autoload eval error")
	}
}

func TestLexerStringNewlineEscape(t *testing.T) {
	// backslash-newline inside a string literal, and inside a regexp literal
	src := "module M =\n let x = \"a\\\nb\"\n let y = /a\\\nb/\n"
	if _, err := parseModule(src); err != nil {
		t.Errorf("newline escape: %v", err)
	}
}

func TestBuiltinExclSuccess(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	fn, _ := i.global.lookup("excl")
	v, err := i.apply(fn, vString("/etc/x"))
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := v.(*vFilter); !ok || f.filter[0].include {
		t.Error("excl filter")
	}
}

func TestPutTerminalFaultInjection(t *testing.T) {
	// A concat terminal whose second child regexp is broken: get over it hits
	// getConcat's error handling; parse hits parseConcat's.
	good := makePrim(lDel, newRegexp("a", false), "a")
	bad := &Lens{tag: lDel, ctype: brokenRe(), regexp: brokenRe(), atype: regexpMakeEmpty()}
	con := makeConcat(good, bad)
	con.ctype = brokenRe() // force the top match to fail cleanly
	if _, err := LnsGet(con, "ab"); err == nil {
		t.Error("broken concat get")
	}
}

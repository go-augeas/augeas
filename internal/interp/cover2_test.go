// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"strings"
	"testing"
)

// brokenRe is a raw regexp that fails to compile, used to exercise the error
// branches that guard regexp matching throughout get/put.
func brokenRe() *Regexp { return newRawRegexp("(") }

func TestBrokenRegexpGet(t *testing.T) {
	// del with a broken ctype: initRegs -> match error
	l := &Lens{tag: lDel, ctype: brokenRe(), regexp: brokenRe(), atype: regexpMakeEmpty()}
	if _, err := LnsGet(l, "x"); err == nil {
		t.Fatal("expected get match error")
	}
	// star whose child ctype is broken: initRegs sets regs, getStar matches child
	star := &Lens{tag: lStar, child: l, ctype: brokenRe(), atype: regexpMakeEmpty()}
	if _, err := LnsGet(star, "x"); err == nil {
		t.Fatal("expected star child match error")
	}
	// square whose child ctype is broken
	inner := &Lens{tag: lConcat, children: []*Lens{l, l, l}, ctype: brokenRe(), atype: regexpMakeEmpty()}
	sq := &Lens{tag: lSquare, child: inner, ctype: brokenRe(), atype: regexpMakeEmpty()}
	if _, err := LnsGet(sq, "x"); err == nil {
		t.Fatal("expected square child match error")
	}
}

func TestBrokenRegexpParse(t *testing.T) {
	l := &Lens{tag: lDel, ctype: brokenRe(), regexp: brokenRe(), atype: regexpMakeEmpty()}
	if _, _, err := lnsParse(l, "x"); err == nil {
		t.Fatal("expected parse match error")
	}
}

func TestBrokenAtypePut(t *testing.T) {
	// A subtree whose child atype is broken -> splitConcat/applies error paths.
	store := makePrim(lStore, newRegexp("[0-9]+", false), "")
	con := &Lens{tag: lConcat, children: []*Lens{store}, ctype: store.ctype, atype: brokenRe()}
	forest := []*Tree{{Value: strptr("5")}}
	if _, err := LnsPut(con, forest, "5"); err == nil {
		t.Fatal("expected put split error")
	}
}

func TestToRE2Escapes(t *testing.T) {
	// backslash before a non-meta char (dropped), before meta (kept)
	if got, _ := toRE2(`\ \.`); got != ` \.` {
		t.Fatalf("escape drop/keep: %q", got)
	}
	// class with POSIX and escapes
	if _, err := toRE2(`[[:alpha:]\]a]`); err != nil {
		t.Fatalf("class posix: %v", err)
	}
	// unterminated class (defensive close)
	if got, _ := toRE2(`[abc`); !strings.HasSuffix(got, "]") {
		t.Fatalf("unterminated class: %q", got)
	}
	// '[' not a POSIX class member
	if _, err := toRE2(`[a[b]`); err != nil {
		t.Fatalf("literal bracket: %v", err)
	}
	// literal meta writeLiteral path via unknown escape that is a meta char
	if got, _ := toRE2(`\+`); got != `\+` {
		t.Fatalf("escaped plus: %q", got)
	}
}

func TestRestrictRE2Paths(t *testing.T) {
	// restrict a raw pattern with a positive class containing control escapes,
	// hex, a range spanning the reserved bytes, and POSIX class.
	r := newRawRegexp(`[\n\t\x00-\xff[:digit:]]`)
	res := restrict(r)
	// must not match the reserved bytes
	if _, ok, _ := res.match("\x03", 0, 1); ok {
		t.Fatal("restricted class matched reserved byte")
	}
	// negated class restriction
	r2 := newRawRegexp(`[^a-]`)
	res2 := restrict(r2)
	if _, ok, _ := res2.match("\x02", 0, 1); ok {
		t.Fatal("restricted negated class matched reserved byte")
	}
	// class that becomes empty after clipping (only reserved bytes)
	r3 := newRawRegexp(`[\x01-\x04]`)
	_ = restrict(r3) // must not panic; emits a valid class
	// "." restriction
	r4 := newRawRegexp(`.`)
	res4 := restrict(r4)
	if _, ok, _ := res4.match("\x03", 0, 1); ok {
		t.Fatal("restricted dot matched reserved byte")
	}
	// trailing backslash in a class
	_ = restrict(newRawRegexp(`[a\`))
}

func TestExpandNocaseEdge(t *testing.T) {
	// letter after escape, class ending with bare dash
	if got := expandNocase(`[a-z-]`); !strings.Contains(got, "A-Z") {
		t.Fatalf("expandNocase dash: %q", got)
	}
	if got := expandNocase(`x`); got != "[xX]" {
		t.Fatalf("expandNocase letter: %q", got)
	}
}

func TestApiErrors(t *testing.T) {
	i := New(srcMap(map[string]string{
		"m": "module M =\n autoload xfm\n let lns = store /a/\n let s = \"x\"\n let xfm = transform lns (incl \"/f\")\n",
		"n": "module N =\n let lns = [ label \"v\" . store /a/ ]\n",
	}))
	// LensValue on a non-lens binding
	if _, err := i.LensValue("M", "s"); err == nil {
		t.Fatal("LensValue non-lens")
	}
	// LensValue missing module
	if _, err := i.LensValue("Zzz", "lns"); err == nil {
		t.Fatal("LensValue missing module")
	}
	// Autoload success
	if _, filt, err := i.Autoload("M"); err != nil || len(filt) != 1 {
		t.Fatalf("autoload: %v %v", filt, err)
	}
	// Autoload on module without autoload
	if _, _, err := i.Autoload("N"); err == nil {
		t.Fatal("autoload none")
	}
	// Autoload missing module
	if _, _, err := i.Autoload("Zzz"); err == nil {
		t.Fatal("autoload missing")
	}
	// Put via public wrapper
	l, _ := i.LensValue("N", "lns")
	if _, err := Put(l, []*Tree{{Label: strptr("v"), Value: strptr("a")}}, "a"); err != nil {
		t.Fatalf("Put: %v", err)
	}
}

func TestLoadModuleErrors(t *testing.T) {
	// import cycle
	i := New(srcMap(map[string]string{
		"a": "module A =\n let x = B.y\n",
		"b": "module B =\n let y = A.x\n",
	}))
	if _, err := i.Lookup("A", "x"); err == nil {
		t.Fatal("expected cycle error")
	}
}

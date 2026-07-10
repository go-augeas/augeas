// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func TestTranslateClassBranches(t *testing.T) {
	cases := []string{
		`[a[b]`,        // literal '[' member (POSIX check fails)
		`[a\wb]`,       // backslash member inside class
		`[abc`,         // unterminated class (defensive close)
		`[]x]`,         // ']' first member is literal
		`[[:digit:]x]`, // POSIX class + member
		`[^]a]`,        // negated with ']' first
	}
	for _, c := range cases {
		_ = toRE2(c)
	}
}

func TestRestrictRE2Branches(t *testing.T) {
	cases := []string{
		`.`,                  // dot
		`[[:digit:]]`,        // positive POSIX
		`[^ab]`,              // negated
		`[^]a]`,              // negated, ']' first
		`[\n\t\r\f\v\a\x0b]`, // control escapes
		`[\x41\xAB\xff]`,     // hex incl A-F
		`[\x]`,               // '\x' with no hex digits (fallback)
		`[a-\xff]`,           // range spanning reserved
		`[\x01-\x04]`,        // only-reserved -> empty after clip
		`[a\`,                // trailing backslash
	}
	for _, c := range cases {
		out := restrictRE2(c)
		if out == "" {
			t.Errorf("restrictRE2(%q) empty", c)
		}
	}
	// decodeClassByte hex A-F and invalid-hex(default 0) via restrict
	_ = restrictRE2(`[\xAf\xZZ]`)
	if hexVal('A') != 10 || hexVal('f') != 15 || hexVal('9') != 9 || hexVal('z') != 0 {
		t.Error("hexVal")
	}
}

func TestExpandNocaseBranches(t *testing.T) {
	for _, c := range []string{`a`, `[a-z]`, `[a-z-]`, `[^]a]`, `[0-9]`, `\da`, `[abc]`} {
		_ = expandNocase(c)
	}
}

func TestAutomataRuneAndOps(t *testing.T) {
	// literal with rune >= 256 exercises the multi-byte UTF-8 expansion in both
	// collectBounds and literalFrag.
	if _, err := regexpMinus(newRawRegexp("Ā"), newRegexp("x", false)); err != nil {
		t.Errorf("multibyte >=256 minus: %v", err)
	}
	if _, err := regexpMinus(newRawRegexp("aĀb"), newRegexp("x", false)); err != nil {
		t.Errorf("multibyte concat minus: %v", err)
	}
	// OpNoMatch operand (empty set); the result language is empty so minus
	// returns an error, but buildNFA still exercises the OpNoMatch branch.
	if _, err := regexpMinus(newRawRegexp(`[^\x00-\x{10ffff}]`), newRegexp("x", false)); err == nil {
		t.Log("empty-set minus unexpectedly succeeded")
	}
	// empty alternation -> OpEmptyMatch -> default epsilon branch
	if _, err := regexpMinus(newRawRegexp("(a|)c"), newRegexp("xyz", false)); err != nil {
		t.Errorf("empty-alt minus: %v", err)
	}
}

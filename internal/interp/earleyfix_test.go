// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

// TestToRE2Anchors covers the context-dependent $/^ translation: literal when
// mid-expression, a line-scoped anchor at (sub)expression boundaries.
func TestToRE2Anchors(t *testing.T) {
	cases := []struct{ in, want string }{
		{`a$`, `a(?m:$)`},     // $ at end -> anchor
		{`a$|b`, `a(?m:$)|b`}, // $ before | -> anchor
		{`(a$)`, `(a(?m:$))`}, // $ before ) -> anchor
		{`$x`, `\$x`},         // literal $ at start (not end)
		{`a$b`, `a\$b`},       // literal $ mid-expression
		{`^a`, `(?m:^)a`},     // ^ at start -> anchor
		{`(^a)`, `((?m:^)a)`}, // ^ after ( -> anchor
		{`a|^b`, `a|(?m:^)b`}, // ^ after | -> anchor
		{`a^b`, `a\^b`},       // literal ^ mid-expression
	}
	for _, c := range cases {
		if got := toRE2(c.in); got != c.want {
			t.Errorf("toRE2(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEnumerate covers the finite/infinite/too-many/parse-error paths of the
// square delimiter word enumerator.
func TestEnumerate(t *testing.T) {
	// finite small language
	words, ok := enumerate(newRegexp(`a|bc`, false), 10)
	if !ok || len(words) != 2 {
		t.Fatalf("finite: ok=%v words=%v", ok, words)
	}
	// optional -> {"", "\""}
	words, ok = enumerate(newRegexp(`"?`, false), 10)
	if !ok || len(words) != 2 {
		t.Fatalf("optional: ok=%v words=%v", ok, words)
	}
	// infinite language -> not ok
	if _, ok := enumerate(newRegexp(`a*`, false), 10); ok {
		t.Fatal("a* should be reported infinite")
	}
	// more than limit words -> not ok
	if _, ok := enumerate(newRegexp(`[a-z]`, false), 10); ok {
		t.Fatal("[a-z] exceeds limit")
	}
	// parse error -> not ok
	if _, ok := enumerate(newRawRegexp(`(`), 10); ok {
		t.Fatal("bad pattern should fail to enumerate")
	}
}

// TestSquarePreciseType covers the nil-operand and infinite-language fallbacks.
func TestSquarePreciseType(t *testing.T) {
	if squarePreciseType(nil, newRegexp(`x`, false)) != nil {
		t.Fatal("nil l1 must yield nil precise type")
	}
	// infinite delimiter language -> fall back to nil (loose concat kept)
	if squarePreciseType(newRegexp(`[a-z]+`, false), newRegexp(`x`, false)) != nil {
		t.Fatal("infinite delimiter must yield nil")
	}
	// finite delimiter -> a precise union is produced
	if squarePreciseType(newRegexp(`"?`, false), newRegexp(`x`, false)) == nil {
		t.Fatal("finite delimiter should yield a precise type")
	}
}

// TestNestedChildPredicate covers the relative-path child predicate, including
// the value-matching and unparseable-predicate branches.
func TestNestedChildPredicate(t *testing.T) {
	build := func() *Tree {
		root := &Tree{}
		e := &Tree{Label: strptr("entry")}
		tm := &Tree{Label: strptr("time")}
		tm.Children = []*Tree{{Label: strptr("minute"), Value: strptr("54")}}
		e.Children = []*Tree{tm}
		root.Children = []*Tree{e}
		return root
	}
	root := build()
	// existence of nested path
	if got := findNodes(root, mustSegs(t, "/entry[time/minute]")); len(got) != 1 {
		t.Fatalf("nested existence: %d", len(got))
	}
	// nested value match (hit)
	if got := findNodes(root, mustSegs(t, "/entry[time/minute = '54']")); len(got) != 1 {
		t.Fatalf("nested value hit: %d", len(got))
	}
	// nested value match (miss)
	if got := findNodes(root, mustSegs(t, "/entry[time/minute = '99']")); len(got) != 0 {
		t.Fatalf("nested value miss: %d", len(got))
	}
	// unparseable child predicate -> no match
	if got := findNodes(root, mustSegs(t, "/entry[time[/minute]")); len(got) != 0 {
		t.Fatalf("bad predicate should match nothing: %d", len(got))
	}
}

func mustSegs(t *testing.T, p string) []pathSeg {
	t.Helper()
	segs, err := parsePath(p)
	if err != nil {
		t.Fatalf("parsePath(%q): %v", p, err)
	}
	return segs
}

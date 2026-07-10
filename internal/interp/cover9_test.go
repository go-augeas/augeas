// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func TestGetPutIdentifiers(t *testing.T) {
	// 'get' and 'put' usable as identifiers (parseAExp tGet/tPut arms).
	src := "module M =\n let f (get:lens) (put:lens) = get . put\n let lns = f (del /a/ \"a\") (del /b/ \"b\")\n"
	i := New(srcMap(map[string]string{"m": src}))
	if _, err := i.LensValue("M", "lns"); err != nil {
		t.Fatalf("get/put idents: %v", err)
	}
}

func TestPutTestExecutes(t *testing.T) {
	// Ensure a put test parses AND runs (covers parseTestExp put arm end-to-end).
	src := "module M =\n let lns = [ key /[a-z]+/ . del /=/ \"=\" . store /[0-9]+/ ]\n" +
		" test lns put \"a=1\" after set \"/a\" \"2\" = \"a=2\"\n"
	i := New(srcMap(map[string]string{"m": src}))
	m, err := i.LoadModule("M")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range m.tests {
		if pe, ok := tt.exp.(*putTestTerm); ok {
			out, err := runPut(i, m.env, mustLens(t, i), "a=1", pe.cmds)
			if err != nil || out != "a=2" {
				t.Fatalf("put test run: %q %v", out, err)
			}
		}
	}
}

func mustLens(t *testing.T, i *interp) *Lens {
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestRecursiveEmptyStar(t *testing.T) {
	// A recursive lens whose star child can match empty text exercises the
	// no-progress guard in the recursive parseStar.
	src := "module M =\n let rec lns = [ store /[a-z]*/ ]* . del /!/ \"!\"\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	safe(func() { LnsGet(l, "!") })
}

func TestRecursiveAmbiguousDedup(t *testing.T) {
	// Two derivations reaching the same end position exercise the dedup in the
	// recursive concat.
	src := "module M =\n let rec lns = ([ key /a/ . store /x/ ] | [ key /a/ . store /x/ ])*\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	safe(func() { LnsGet(l, "ax") })
}

func TestDfaCombinatorSweep(t *testing.T) {
	pairs := [][2]string{
		{"a(b|c)d", "abd"},
		{"(x|y|z)+", "y"},
		{"a?b?c?", "b"},
		{"[a-z]*[0-9]", "a5"},
		{"(foo|foobar)", "foo"},
		{"a|ab|abc", "ab"},
	}
	for _, p := range pairs {
		_, _ = regexpMinus(newRegexp(p[0], false), newRegexp(p[1], false))
	}
}

func TestEvalConcatNonCombinable(t *testing.T) {
	// concatValues where neither side is string/regexp/lens/filter
	if _, err := concatValues(vUnit{}, vUnit{}); err == nil {
		t.Error("concat unit unit")
	}
	// minusValues error via broken raw operand
	if _, err := minusValues(&vRegexp{re: newRawRegexp("(")}, vString("a")); err == nil {
		t.Error("minus broken")
	}
}

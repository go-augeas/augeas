// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func TestTranslatePosixFail(t *testing.T) {
	// '[:' that is not a valid POSIX class (no ':]') -> literal '[' branch
	for _, c := range []string{`[[:x]]`, `[a[:9]b]`, `[[:]]`} {
		_ = toRE2(c)
	}
	// restrictRE2 with a positive class containing '[:' that is not POSIX
	_ = restrictRE2(`[a[:z]b]`)
	// restrictRE2 positive class (else branch) and negated
	_ = restrictRE2(`[abc]`)
	_ = restrictRE2(`[^abc]`)
}

func TestParserBattery2(t *testing.T) {
	bad := []string{
		"module M =\n let rec x y = /a/\n",         // rec: expect '=' fails
		"module M =\n test x put = after y = z\n",  // put arg not an aexp
		"module M =\n test x put y z\n",            // missing 'after'
		"module M =\n let x (p:(zzz)) = /a/\n",     // paren type inner bad
		"module M =\n let x = let y = in z\n",      // let-in exp missing
		"module M =\n let x = let y = /a/ in\n",    // let-in body missing
		"module M =\n let x = { \"a\" { \"b\" }\n", // tree: outer rbrace missing
		"module M =\n let x = { \"a\" = \"b\"\n",   // tree rbrace missing
	}
	for _, s := range bad {
		if _, err := parseModule(s); err == nil {
			t.Errorf("expected error %q", s)
		}
	}
	// valid put test + paren-expression that is not a param list
	for _, s := range []string{
		"module M =\n test x put \"a\" after set \"/p\" \"v\" = \"b\"\n let x = del /a/ \"a\"\n",
		"module M =\n let f = (/a/)\n",
	} {
		if _, err := parseModule(s); err != nil {
			t.Errorf("unexpected error %q: %v", s, err)
		}
	}
}

func TestParseTreeConstDirect(t *testing.T) {
	// parseTreeConst on a parser not positioned at '{' returns an error.
	lx := newLexer("/a/")
	toks, _ := lx.tokens()
	p := &parser{toks: toks}
	if _, err := p.parseTreeConst(); err == nil {
		t.Error("parseTreeConst non-brace")
	}
}

func TestTreecmdPredicateAndParent(t *testing.T) {
	// findChildren predChild without a matching child value -> filtered out
	r := &Tree{Children: []*Tree{{Label: strptr("a"), Children: []*Tree{{Label: strptr("k"), Value: strptr("v")}}}}}
	segs, _ := parsePath("/a[k = 'other']")
	if len(findNodes(r, segs)) != 0 {
		t.Error("child value predicate")
	}
	// rm on a top-level node uses parentOf(root)
	r2 := &Tree{Children: []*Tree{{Label: strptr("x")}}}
	if err := treeCmdRm(r2, "/x"); err != nil || len(r2.Children) != 0 {
		t.Errorf("rm top: %v", err)
	}
	// insa where path matches nothing -> error (parent nil path)
	if err := treeCmdIns(r2, "z", "/none", false); err == nil {
		t.Error("ins no match")
	}
}

func TestApiAutoloadNoBinding(t *testing.T) {
	i := New(srcMap(map[string]string{
		"m": "module M =\n autoload xfm\n let y = /a/\n",
	}))
	if _, _, err := i.Autoload("M"); err == nil {
		t.Error("autoload missing binding")
	}
}

func TestLexerCommentNewline(t *testing.T) {
	src := "module M =\n(* a\n b *)\n let x = /a/\n"
	if _, err := parseModule(src); err != nil {
		t.Errorf("comment newline: %v", err)
	}
}

func TestRegexpBuildCached(t *testing.T) {
	// A broken regexp: first build fails, second call returns the cached error.
	r := newRawRegexp("(")
	if _, _, err := r.match("x", 0, 1); err == nil {
		t.Error("first match should error")
	}
	if _, _, err := r.match("x", 0, 1); err == nil {
		t.Error("cached error expected")
	}
}

func TestBuiltinLensTypeAllNonLens(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	for _, n := range []string{"lens_atype", "lens_ktype", "lens_vtype"} {
		fn, _ := i.global.lookup(n)
		if _, err := i.apply(fn, vString("x")); err == nil {
			t.Errorf("%s non-lens", n)
		}
	}
}

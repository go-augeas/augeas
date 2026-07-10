// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func TestParserErrorBattery(t *testing.T) {
	bad := []string{
		"module",                                  // missing name
		"module 1",                                // name not UIDENT
		"module M",                                // missing =
		"module M = autoload",                     // autoload missing lident
		"module M = let",                          // let missing lident
		"module M = let x",                        // missing =
		"module M = let x =",                      // missing exp
		"module M = let rec",                      // rec missing lident
		"module M = let rec x",                    // rec missing =
		"module M = let rec x =",                  // rec missing exp
		"module M = test",                         // test missing exp
		"module M = test /a/",                     // missing get/put
		"module M = test /a/ get",                 // get missing exp
		"module M = test /a/ get \"x\" =",         // missing result
		"module M = test /a/ put /b/",             // put missing after
		"module M = test /a/ put /b/ after",       // put missing cmds
		"module M = let x = ( /a/",                // missing rparen
		"module M = let x = [ /a/",                // missing rbracket
		"module M = let x = /a/ .",                // dangling concat
		"module M = let x = /a/ |",                // dangling union
		"module M = let x = /a/ -",                // dangling minus
		"module M = let x (p",                     // param missing colon/type
		"module M = let x (p:zzz) = /a/",          // bad type keyword
		"module M = let x (p:string = /a/",        // param missing rparen
		"module M = let x = let y = /a/",          // let-in missing 'in'
		"module M = let x = let y = /a/ in",       // let-in missing body
		"module M = let x = { \"a\" = }",          // tree value not a string
		"module M = let x = { \"a\"",              // tree missing rbrace
		"module M = let x = {",                    // tree branch then EOF
		"module M = let x = /a/ trailing extra )", // trailing tokens after decl? actually parse error
	}
	for _, s := range bad {
		if _, err := parseModule(s); err == nil {
			t.Errorf("expected parse error for %q", s)
		}
	}
}

func TestParserOkForms(t *testing.T) {
	ok := []string{
		"module M =\n let x = /a/\n",
		"module M =\n autoload xfm\n let x = /a/\n let xfm = transform x (incl \"/f\")\n",
		"module M =\n test M.x get \"a\" = ?\n let x = del /a/ \"a\"\n", // trPrint
		"module M =\n test x get \"a\" = *\n let x = del /a/ \"a\"\n",   // trExn
		"module M =\n let x (p:string -> lens) = /a/\n",                 // arrow type
		"module M =\n let x (p:(lens)) = /a/\n",                         // paren type
		"module M =\n let f (get:lens) = get\n",                         // id can be 'get'
		"module M =\n let x = { \"a\" = \"b\" { \"c\" } }\n",            // nested tree value
		"module M =\n let x = /a/ | { \"t\" }\n",                        // union with tree const
	}
	for _, s := range ok {
		if _, err := parseModule(s); err != nil {
			t.Errorf("unexpected parse error for %q: %v", s, err)
		}
	}
}

func TestTreeCmdBadPaths(t *testing.T) {
	r := &Tree{Children: []*Tree{{Label: strptr("a"), Value: strptr("1")}}}
	bad := "/a[bad"
	if err := treeCmdSet(r, bad, "x"); err == nil {
		t.Error("set bad path")
	}
	if err := treeCmdClear(r, bad); err == nil {
		t.Error("clear bad path")
	}
	if err := treeCmdRm(r, bad); err == nil {
		t.Error("rm bad path")
	}
	if err := treeCmdIns(r, "z", bad, false); err == nil {
		t.Error("ins bad path")
	}
	// create path through a bad segment
	if err := treeCmdSet(r, "/new/x[bad", "y"); err == nil {
		t.Error("set create bad path")
	}
}

func TestTreeCmdNativeErrors(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	call := func(name string, args ...Value) error {
		v, _ := i.global.lookup(name)
		var err error
		for _, a := range args {
			v, err = i.apply(v, a)
			if err != nil {
				return err
			}
		}
		return nil
	}
	tr := &vTree{forest: []*Tree{{Label: strptr("a")}}}
	// non-string path arg
	if call("set", vUnit{}, vString("v"), tr) == nil {
		t.Error("set non-string path")
	}
	if call("set", vString("/a"), vUnit{}, tr) == nil {
		t.Error("set non-string value")
	}
	if call("rm", vUnit{}, tr) == nil {
		t.Error("rm non-string")
	}
	if call("clear", vUnit{}, tr) == nil {
		t.Error("clear non-string")
	}
	if call("insa", vUnit{}, vString("/a"), tr) == nil {
		t.Error("insa non-string label")
	}
	if call("insb", vString("z"), vUnit{}, tr) == nil {
		t.Error("insb non-string path")
	}
	// non-tree last argument
	if call("rm", vString("/a"), vUnit{}) == nil {
		t.Error("rm non-tree")
	}
	// bad path through native
	if call("set", vString("/a[bad"), vString("v"), tr) == nil {
		t.Error("set native bad path")
	}
}

func TestEvalMoreErrors(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	e := newEnv(i.global)
	// unknown term type not evaluable: use a bindTerm (not an expression)
	if _, err := i.eval(&bindTerm{name: "x"}, e); err == nil {
		t.Error("bindTerm not evaluable")
	}
	// app of non-function
	if _, err := i.evalApp(&binopTerm{tag: opApp, left: &stringTerm{value: "x"}, right: &stringTerm{value: "y"}}, e); err == nil {
		t.Error("app non-func")
	}
	// concat error propagation (left evals but is bad combo)
	if _, err := i.eval(&binopTerm{tag: opConcat, left: &unitTerm{}, right: &unitTerm{}}, e); err == nil {
		t.Error("concat unit unit")
	}
	// minus with non-regexp
	if _, err := i.eval(&binopTerm{tag: opMinus, left: &unitTerm{}, right: &unitTerm{}}, e); err == nil {
		t.Error("minus unit")
	}
	// left eval error surfaces
	if _, err := i.eval(&binopTerm{tag: opConcat, left: &identTerm{name: "nope"}, right: &stringTerm{value: "y"}}, e); err == nil {
		t.Error("concat left err")
	}
	// right eval error surfaces
	if _, err := i.eval(&binopTerm{tag: opConcat, left: &stringTerm{value: "y"}, right: &identTerm{name: "nope"}}, e); err == nil {
		t.Error("concat right err")
	}
}

func TestRegexIterMaybeRaw(t *testing.T) {
	// iter/maybe on a raw regexp (min==max, and {n,m}) exercises those branches
	raw := newRawRegexp("[a-c]")
	if regexpIter(raw, 2, 2) == nil {
		t.Error("iter min==max raw")
	}
	if regexpIter(raw, 2, 4) == nil {
		t.Error("iter n,m raw")
	}
	if regexpMaybe(raw) == nil {
		t.Error("maybe raw")
	}
	// non-raw min==max / n,m
	r := newRegexp("a", false)
	if regexpIter(r, 2, 2) == nil || regexpIter(r, 2, 4) == nil {
		t.Error("iter non-raw")
	}
	// nil operands
	if regexpIter(nil, 0, -1) != nil || regexpMaybe(nil) != nil {
		t.Error("nil iter/maybe")
	}
	if regexpConcatN(nil) != nil || regexpUnionN(nil) != nil {
		t.Error("empty concat/union")
	}
}

func TestAutomataOps(t *testing.T) {
	// NoMatch / repeat / anychar via minus operands
	if _, err := regexpMinus(newRegexp("a{2,3}", false), newRegexp("aa", false)); err != nil {
		t.Errorf("repeat minus: %v", err)
	}
	// concat empty subexpr
	if _, err := regexpMinus(newRegexp("(a|)b", false), newRegexp("b", false)); err != nil {
		t.Errorf("empty-alt minus: %v", err)
	}
}

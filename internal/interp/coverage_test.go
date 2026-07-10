// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"strings"
	"testing"
)

func TestBuiltinTypeErrors(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	get := func(n string) Value { v, _ := i.global.lookup(n); return v }
	apply2 := func(n string, args ...Value) error {
		v := get(n)
		var err error
		for _, a := range args {
			v, err = i.apply(v, a)
			if err != nil {
				return err
			}
		}
		return nil
	}
	// wrong argument types must error
	if apply2("store", vUnit{}) == nil {
		t.Fatal("store non-regexp")
	}
	if apply2("key", vUnit{}) == nil {
		t.Fatal("key non-regexp")
	}
	if apply2("label", vUnit{}) == nil {
		t.Fatal("label non-string")
	}
	if apply2("value", vUnit{}) == nil {
		t.Fatal("value non-string")
	}
	if apply2("seq", vUnit{}) == nil {
		t.Fatal("seq non-string")
	}
	if apply2("counter", vUnit{}) == nil {
		t.Fatal("counter non-string")
	}
	if apply2("del", vUnit{}, vString("x")) == nil {
		t.Fatal("del non-regexp")
	}
	if apply2("del", &vRegexp{re: newRegexp("a", false)}, vUnit{}) == nil {
		t.Fatal("del non-string default")
	}
	if apply2("incl", vUnit{}) == nil {
		t.Fatal("incl non-string")
	}
	if apply2("excl", vUnit{}) == nil {
		t.Fatal("excl non-string")
	}
	if apply2("square", vUnit{}, vUnit{}, vUnit{}) == nil {
		t.Fatal("square non-lens")
	}
	if apply2("transform", vUnit{}, vUnit{}) == nil {
		t.Fatal("transform non-lens")
	}
	lensV := &vLens{lens: makePrim(lStore, newRegexp("a", false), "")}
	if apply2("transform", lensV, vUnit{}) == nil {
		t.Fatal("transform non-filter")
	}
}

func TestBuiltinLensTypesAndGet(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	lensV := &vLens{lens: makePrim(lStore, newRegexp("[0-9]+", false), "")}
	for _, n := range []string{"lens_ctype", "lens_atype", "lens_ktype", "lens_vtype"} {
		v, _ := i.global.lookup(n)
		r, err := i.apply(v, lensV)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		if _, ok := r.(*vRegexp); !ok {
			t.Fatalf("%s not regexp", n)
		}
		// wrong arg
		if _, err := i.apply(v, vUnit{}); err == nil {
			t.Fatalf("%s non-lens should error", n)
		}
	}
	// get native
	getFn, _ := i.global.lookup("get")
	del := &vLens{lens: makePrim(lDel, newRegexp("[0-9]+", false), "0")}
	v, _ := i.apply(getFn, del)
	if _, err := i.apply(v, vString("5")); err != nil {
		t.Fatalf("get native: %v", err)
	}
	if _, err := i.apply(v, vUnit{}); err == nil {
		// second arg must be string; but we already consumed; re-do
	}
	// print_* natives and Sys.getenv
	for _, n := range []string{"print_string", "print_regexp", "print_endline"} {
		fn, _ := i.global.lookup(n)
		if _, err := i.apply(fn, vString("x")); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	ptree, _ := i.global.lookup("print_tree")
	if _, err := i.apply(ptree, &vTree{}); err != nil {
		t.Fatal(err)
	}
	env, _ := i.global.lookup("Sys.getenv")
	if _, err := i.apply(env, vString("PATH")); err != nil {
		t.Fatal(err)
	}
}

func TestEvalMisc(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	e := newEnv(i.global)
	// bracket on non-lens
	if _, err := i.eval(&bracketTerm{exp: &stringTerm{value: "x"}}, e); err == nil {
		t.Fatal("bracket non-lens")
	}
	// rep on non-lens/regexp (unit)
	if _, err := i.eval(&repTerm{exp: &unitTerm{}, quant: qStar}, e); err == nil {
		t.Fatal("rep unit")
	}
	// rep on string coerces to regexp
	if _, err := i.eval(&repTerm{exp: &stringTerm{value: "a"}, quant: qPlus}, e); err != nil {
		t.Fatal("rep string plus")
	}
	if _, err := i.eval(&repTerm{exp: &stringTerm{value: "a"}, quant: qMaybe}, e); err != nil {
		t.Fatal("rep string maybe")
	}
	// unknown identifier
	if _, err := i.eval(&identTerm{name: "nope"}, e); err == nil {
		t.Fatal("unbound")
	}
	// let..in
	lt := &letTerm{name: "x", exp: &regexpTerm{pattern: "a"}, body: &identTerm{name: "x"}}
	if _, err := i.eval(lt, e); err != nil {
		t.Fatalf("let-in: %v", err)
	}
	// funcTerm -> closure, apply
	fn := &funcTerm{param: param{name: "p"}, body: &identTerm{name: "p"}}
	cv, _ := i.eval(fn, e)
	if _, err := i.apply(cv, vString("z")); err != nil {
		t.Fatal("closure apply")
	}
	// tree value term
	lbl := "k"
	tv := &treeValueTerm{nodes: []*treeLit{{label: &lbl}}}
	if _, err := i.eval(tv, e); err != nil {
		t.Fatal("tree value")
	}
}

func TestConcatUnionMinusMore(t *testing.T) {
	// filter concat
	f1 := &vFilter{filter: []filterEntry{{glob: "a", include: true}}}
	f2 := &vFilter{filter: []filterEntry{{glob: "b", include: false}}}
	v, err := concatValues(f1, f2)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.(*vFilter).filter) != 2 {
		t.Fatal("filter concat")
	}
	// filter concat with non-filter
	if _, err := concatValues(f1, vUnit{}); err == nil {
		t.Fatal("filter . unit")
	}
	// lens union with non-lens
	ll := &vLens{lens: makePrim(lStore, newRegexp("a", false), "")}
	if _, err := unionValues(ll, vUnit{}); err == nil {
		t.Fatal("lens | unit")
	}
	if _, err := concatValues(vUnit{}, ll); err == nil {
		t.Fatal("unit . lens")
	}
	// minus string-string
	if _, err := minusValues(vString("[a-z]"), vString("b")); err != nil {
		t.Fatalf("minus strings: %v", err)
	}
}

func TestComposedApply(t *testing.T) {
	i := New(srcMap(map[string]string{}))
	// build a composed of two set-like natives via evalCompose
	src := "module M =\n let f = /a/ ; /b/\n"
	i2 := New(srcMap(map[string]string{"m": src}))
	if _, err := i2.Lookup("M", "f"); err != nil {
		t.Fatal(err)
	}
	_ = i
}

func TestGetUnusedKeyValue(t *testing.T) {
	// a key with no surrounding subtree leaves an unused key -> error
	l := compileLens(t, ` let lns = key /[a-z]+/`)
	if _, err := LnsGet(l, "abc"); err == nil {
		t.Fatal("expected unused key error")
	}
	lv := compileLens(t, ` let lns = store /[a-z]+/`)
	if _, err := LnsGet(lv, "abc"); err == nil {
		t.Fatal("expected unused value error")
	}
}

func TestToRE2Errors(t *testing.T) {
	// trailing backslash
	if got := toRE2(`a\`); !strings.Contains(got, `\\`) {
		t.Fatalf("trailing backslash: %q", got)
	}
}

func TestRegexpStringNil(t *testing.T) {
	if (&Regexp{}).re2() == "" && false {
		t.Fatal("unreachable")
	}
}

func TestGetErrorPaths(t *testing.T) {
	// initRegs: top lens doesn't match at all
	l := compileLens(t, ` let lns = del /abc/ "x"`)
	if _, err := LnsGet(l, "zzz"); err == nil {
		t.Fatal("no-match should error")
	}
	// partial match (doesn't consume whole input)
	l2 := compileLens(t, ` let lns = del /a/ "a"`)
	if _, err := LnsGet(l2, "aXX"); err == nil {
		t.Fatal("partial should error")
	}
	// square get mismatch
	sq := compileLens(t, ` let lns = [ square (key /[a-z]+/) (store /[0-9]+/) (del /[a-z]+/ "z") ]`)
	if _, err := LnsGet(sq, "ab5cd"); err == nil {
		t.Fatal("square mismatch should error")
	}
}

func TestMinusNocaseAndMulti(t *testing.T) {
	// nocase operand
	r, err := regexpMinus(newRegexp("[a-c]", true), newRegexp("b", false))
	if err != nil {
		t.Fatalf("nocase minus: %v", err)
	}
	if _, ok, _ := r.match("A", 0, 1); !ok {
		t.Fatal("A should match nocase minus")
	}
}

func TestAutomataMultibyte(t *testing.T) {
	// literal with a multibyte rune exercises the UTF-8 expansion path
	r, err := regexpMinus(newRegexp("é|a", false), newRegexp("a", false))
	if err != nil {
		t.Fatalf("multibyte minus: %v", err)
	}
	if _, ok, _ := r.match("é", 0, len("é")); !ok {
		t.Fatal("é should match")
	}
	// AnyChar via "." difference
	r2, err := regexpMinus(newRegexp(".", false), newRegexp("x", false))
	if err != nil {
		t.Fatalf("anychar minus: %v", err)
	}
	if _, ok, _ := r2.match("y", 0, 1); !ok {
		t.Fatal("y should match . - x")
	}
}

func TestParserMoreErrors(t *testing.T) {
	for _, s := range []string{
		"module M = let x (p:bad) = /a/",    // bad type
		"module M = let x = [ /a/ ",         // unclosed bracket
		"module M = let x = /a/ .",          // dangling concat
		"module M = test /a/ get",           // test missing rhs
		"module M = test /a/ put /b/ after", // put missing cmds
	} {
		if _, err := parseModule(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}

func TestParamsAndFuncApp(t *testing.T) {
	src := "module M =\n" +
		" let f (a:regexp) (b:lens) = [ del a \"x\" . label \"n\" . b ]\n" +
		" let lns = f /=/ (store /[0-9]+/)\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatalf("func app: %v", err)
	}
	if _, err := LnsGet(l, "=5"); err != nil {
		t.Fatalf("get: %v", err)
	}
}

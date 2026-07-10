// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "testing"

func TestParserBattery3(t *testing.T) {
	bad := []string{
		"module M =\n test lns get \"a\" \"b\"\n let lns = del /a/ \"a\"\n", // test missing '='
		"module M =\n let x (/a/) = /b/\n",                                  // '(' not a param -> break, then '(' where '=' expected
		"module M =\n let x = let = /a/ in z\n",                             // let-in name missing
		"module M =\n let x = let y (p:zzz) = /a/ in z\n",                   // let-in param type bad
		"module M =\n let x = del [\n",                                      // app arg bracket exp err
		"module M =\n let x = [ )\n",                                        // bracket exp err
		"module M =\n let x = { \"a\" { \"b\" \"c\"\n",                      // tree child rbrace missing
		"module M =\n let x = )\n",                                          // rexp aexp err at decl start
	}
	for _, s := range bad {
		if _, err := parseModule(s); err == nil {
			t.Errorf("expected error %q", s)
		}
	}
}

func TestRemoveChildNotFound(t *testing.T) {
	p := &Tree{Children: []*Tree{{Label: strptr("a")}}}
	if removeChild(p, &Tree{Label: strptr("b")}) {
		t.Error("removeChild should report not-found")
	}
}

func TestTreeCmdInsbBadPath(t *testing.T) {
	r := &Tree{Children: []*Tree{{Label: strptr("a")}}}
	if err := treeCmdIns(r, "z", "/a[bad", true); err == nil {
		t.Error("insb bad path")
	}
}

func TestBuiltinLensTypePartial(t *testing.T) {
	// lens_ctype applied to a lens value succeeds (covers the ok path fully).
	i := New(srcMap(map[string]string{"m": "module M =\n let x = store /a/\n"}))
	l, _ := i.LensValue("M", "x")
	fn, _ := i.global.lookup("lens_ctype")
	if _, err := i.apply(fn, &vLens{lens: l}); err != nil {
		t.Fatalf("lens_ctype: %v", err)
	}
}

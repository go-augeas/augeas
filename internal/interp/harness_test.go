// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// distDir locates the embedded lens corpus relative to the package.
const distDir = "../../lenses/dist"

func corpusSource() Source {
	return func(base string) (string, bool) {
		for _, p := range []string{
			filepath.Join(distDir, base+".aug"),
			filepath.Join(distDir, "tests", base+".aug"),
		} {
			b, err := os.ReadFile(p)
			if err == nil {
				return string(b), true
			}
		}
		return "", false
	}
}

// testModuleNames returns the real module names of all test_*.aug files, read
// from each file's `module` declaration (the filename is a lowercased form and
// cannot be inverted reliably).
func testModuleNames(t *testing.T) []string {
	entries, err := os.ReadDir(filepath.Join(distDir, "tests"))
	if err != nil {
		t.Fatalf("read tests dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "test_") || !strings.HasSuffix(n, ".aug") {
			continue
		}
		src, err := os.ReadFile(filepath.Join(distDir, "tests", n))
		if err != nil {
			t.Fatalf("read %s: %v", n, err)
		}
		m, err := parseModule(string(src))
		if err != nil {
			t.Logf("PARSEERR %s: %v", n, err)
			continue
		}
		names = append(names, m.name)
	}
	sort.Strings(names)
	return names
}

type tally struct {
	pass, fail, skip int
}

func runOneTest(i *interp, e *env, tt *testTerm) (ok bool, skipped bool, msg string) {
	switch te := tt.exp.(type) {
	case *getTestTerm:
		lv, err := i.eval(te.lens, e)
		if err != nil {
			return false, false, "lens eval: " + err.Error()
		}
		lens, ok := lv.(*vLens)
		if !ok {
			return false, false, "test lens is not a lens"
		}
		av, err := i.eval(te.arg, e)
		if err != nil {
			return false, false, "arg eval: " + err.Error()
		}
		s, ok := av.(vString)
		if !ok {
			return false, false, "test arg is not a string"
		}
		forest, gerr := LnsGet(lens.lens, string(s))
		switch tt.tag {
		case trExn:
			if gerr != nil {
				return true, false, ""
			}
			return false, false, "expected failure but get succeeded"
		case trPrint:
			if gerr != nil {
				return false, false, "get: " + gerr.Error()
			}
			return true, false, ""
		default:
			if gerr != nil {
				return false, false, "get: " + gerr.Error()
			}
			rv, err := i.eval(tt.result, e)
			if err != nil {
				return false, false, "result eval: " + err.Error()
			}
			rt, ok := rv.(*vTree)
			if !ok {
				return false, false, "expected result not a tree"
			}
			if treesEqual(forest, rt.forest) {
				return true, false, ""
			}
			return false, false, fmt.Sprintf("tree mismatch (got %d nodes, want %d)",
				len(forest), len(rt.forest))
		}
	case *putTestTerm:
		return false, true, "put test (not yet run)"
	}
	return false, true, "unknown test kind"
}

func TestCorpus(t *testing.T) {
	names := testModuleNames(t)
	var total tally
	perModule := map[string]tally{}
	var loadErrs []string
	var failLog []string
	verbose := os.Getenv("AUG_VERBOSE") != ""

	for _, name := range names {
		i := New(corpusSource())
		m, err := i.LoadModule(name)
		if err != nil {
			loadErrs = append(loadErrs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		var mt tally
		for _, tt := range m.tests {
			ok, skipped, msg := runOneTest(i, m.env, tt)
			switch {
			case skipped:
				mt.skip++
				total.skip++
			case ok:
				mt.pass++
				total.pass++
			default:
				mt.fail++
				total.fail++
				failLog = append(failLog, fmt.Sprintf("FAIL %s:%d: %s", name, tt.line, firstLine(msg)))
			}
		}
		perModule[name] = mt
	}

	// Report per-module and grand totals.
	if verbose {
		_ = os.WriteFile("/tmp/aug_fails.txt", []byte(strings.Join(failLog, "\n")+"\n"), 0o644)
		_ = os.WriteFile("/tmp/aug_loaderrs.txt", []byte(strings.Join(loadErrs, "\n")+"\n"), 0o644)
		var pm []string
		for _, name := range names {
			mt := perModule[name]
			pm = append(pm, fmt.Sprintf("%s %d %d %d", name, mt.pass, mt.fail, mt.skip))
		}
		_ = os.WriteFile("/tmp/aug_permodule.txt", []byte(strings.Join(pm, "\n")+"\n"), 0o644)
	}
	t.Logf("=== Load errors: %d ===", len(loadErrs))
	getAssertions := total.pass + total.fail
	t.Logf("=== GET assertions: %d passing, %d failing / %d total (%.1f%%); put-skipped: %d ===",
		total.pass, total.fail, getAssertions, pct(total.pass, getAssertions), total.skip)
	t.Logf("=== Modules: %d attempted, %d failed to load ===", len(names), len(loadErrs))
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

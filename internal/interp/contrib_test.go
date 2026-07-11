// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors
//
// CI-gated corpus test for the go-augeas-original contrib lenses under
// ../../lenses/contrib. These lenses are LGPL v2+ (matching the Augeas corpus,
// see lenses/contrib/NOTICE and their own headers), separate from this
// package's BSD-3 code. They are validated exactly like TestCorpus validates
// the verbatim upstream mirror: by running each lens's own inline
// `test ... get ... =` and `test ... put ... after ... =` assertions through
// the pure-Go interpreter. A regression in either lens fails CI here.

package interp

import (
	"os"
	"path/filepath"
	"testing"
)

// contribDir locates the go-augeas-original contrib lenses relative to the
// package.
const contribDir = "../../lenses/contrib"

// contribSource resolves a contrib lens from ../../lenses/contrib first, then
// falls back to the embedded dist corpus so imports (Util, IniFile, Sep, Rx,
// ...) resolve, mirroring the production corpusSource resolver.
func contribSource() Source {
	corpus := corpusSource()
	return func(base string) (string, bool) {
		if b, err := os.ReadFile(filepath.Join(contribDir, base+".aug")); err == nil {
			return string(b), true
		}
		return corpus(base)
	}
}

// contribLens names each contrib module and the get/put test counts its .aug
// file asserts, so the counts themselves are guarded against silent drift.
type contribLens struct {
	module  string
	wantGet int
	wantPut int
}

func TestContribCorpus(t *testing.T) {
	lenses := []contribLens{
		{module: "Wireguard", wantGet: 2, wantPut: 2},
		{module: "Rclone", wantGet: 1, wantPut: 2},
		{module: "Caddyfile", wantGet: 10, wantPut: 3},
	}

	for _, cl := range lenses {
		t.Run(cl.module, func(t *testing.T) {
			i := New(contribSource())
			m, err := i.LoadModule(cl.module)
			if err != nil {
				t.Fatalf("load %s: %v", cl.module, err)
			}
			if len(m.tests) == 0 {
				t.Fatalf("%s: no inline tests found", cl.module)
			}

			var getN, putN int
			for _, tt := range m.tests {
				_, isPut := tt.exp.(*putTestTerm)
				ok, skipped, msg := runOneTest(i, m.env, tt)
				if skipped {
					t.Fatalf("%s:%d unexpected skip: %s", cl.module, tt.line, msg)
				}
				if !ok {
					t.Errorf("%s:%d FAIL: %s", cl.module, tt.line, msg)
					continue
				}
				if isPut {
					putN++
				} else {
					getN++
				}
			}
			if getN != cl.wantGet || putN != cl.wantPut {
				t.Errorf("%s: got %d get + %d put passing, want %d get + %d put",
					cl.module, getN, putN, cl.wantGet, cl.wantPut)
			}
			t.Logf("%s: %d/%d tests pass (get=%d put=%d)",
				cl.module, getN+putN, len(m.tests), getN, putN)
		})
	}
}

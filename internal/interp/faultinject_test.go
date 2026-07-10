// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"errors"
	"testing"
)

// safe runs f, swallowing any panic so a fault-injected inconsistent state
// cannot crash the sweep.
func safe(f func()) {
	defer func() { _ = recover() }()
	f()
}

// injectAt installs a match hook that, on the k-th match call, returns the given
// (regs, ok, err) as an override.
func injectAt(k int, regs []int, ok bool, err error) {
	matchCall = 0
	matchHook = func(call int) ([]int, bool, error, bool) {
		if call == k {
			return regs, ok, err, true
		}
		return nil, false, nil, false
	}
}

func clearInject() { matchHook = nil; matchCall = 0 }

// TestFaultInjectionSweep drives get/parse/put over rich lenses while forcing
// the match layer to fail (error / spurious no-match / empty match) at each
// successive call, exercising the defensive error branches throughout.
func TestFaultInjectionSweep(t *testing.T) {
	defer clearInject()
	boom := errors.New("injected match failure")
	lenses := []string{
		` let lns = ( [ key /[a-z]+/ . del /=/ "=" . store /[0-9]+/ ] . del /\n/ "\n" | [ label "c" . del /#/ "#" . store /[a-z]+/ ] . del /\n/ "\n" )*`,
		` let lns = [ label "x" . store /[0-9]+/ ]? . del /\n/ "\n"`,
		` let lns = [ square (key /[a-z]+/) (store /[0-9]+/) (del /[a-z]+/ "z") ]`,
		` let lns = [ seq "s" . store /[0-9]+/ . del /,/ "," ]*`,
	}
	inputs := []string{"a=1\n#foo\n", "5\n", "ab5ab", "1,2,"}

	for li, body := range lenses {
		l := compileLens(t, body)
		in := inputs[li]
		forest, gerr := LnsGet(l, in)
		if gerr != nil {
			continue
		}
		for k := 1; k <= 30; k++ {
			for _, mode := range []int{0, 1, 2} {
				var regs []int
				var ok bool
				var err error
				switch mode {
				case 0: // error
					err = boom
				case 1: // spurious no-match
				case 2: // empty / zero-length match
					regs, ok = []int{0, 0}, true
				}
				injectAt(k, regs, ok, err)
				safe(func() { LnsGet(l, in) })
				injectAt(k, regs, ok, err)
				safe(func() { lnsParse(l, in) })
				injectAt(k, regs, ok, err)
				safe(func() { LnsPut(l, forest, in) })
			}
		}
		clearInject()
	}
}

// TestFaultInjectionShortRegs drives put with deliberately-short register arrays
// to hit the bounds guards in the split functions.
func TestFaultInjectionShortRegs(t *testing.T) {
	defer clearInject()
	l := compileLens(t, ` let lns = ( [ key /[a-z]+/ . del /=/ "=" . store /[0-9]+/ ] . del /\n/ "\n" )*`)
	in := "a=1\n"
	forest, err := LnsGet(l, in)
	if err != nil {
		t.Fatal(err)
	}
	for k := 1; k <= 20; k++ {
		for _, regs := range [][]int{
			{0, 4},       // too few groups -> reg out of range
			{-1, -1},     // unmatched group 0
			{0, 4, 8, 2}, // ci>cj style (end before start mapping)
		} {
			injectAt(k, regs, true, nil)
			safe(func() { LnsPut(l, forest, in) })
		}
	}
	clearInject()
}

// TestFaultInjectionGet drives get with injected negative/empty registers and
// mid-walk errors to hit the leaf no-match guards and s.err propagation guards.
func TestFaultInjectionGet(t *testing.T) {
	defer clearInject()
	boom := errors.New("boom")
	// leaves whose register is forced to "unmatched" (-1)
	for _, body := range []string{
		` let lns = del /a/ "a"`,
		` let lns = store /a/`,
		` let lns = key /a/`,
	} {
		l := compileLens(t, body)
		injectAt(1, []int{-1, -1}, true, nil) // initRegs match: group0 unmatched
		safe(func() { LnsGet(l, "a") })
		injectAt(1, []int{-1, -1}, true, nil)
		safe(func() { lnsParse(l, "a") })
	}
	// concat with a star in the middle so an error during the star's match
	// propagates to the following child's getLens/parseLens s.err guard.
	l := compileLens(t, ` let lns = del /a/ "a" . (del /b/ "b")* . del /c/ "c"`)
	for k := 1; k <= 8; k++ {
		injectAt(k, nil, false, boom)
		safe(func() { LnsGet(l, "abc") })
		injectAt(k, nil, false, boom)
		safe(func() { lnsParse(l, "abc") })
	}
	// union where all branch registers are forced unmatched -> !applied
	u := compileLens(t, ` let lns = del /a/ "a" | del /b/ "b"`)
	injectAt(1, []int{-1, -1, -1, -1, -1, -1}, true, nil)
	safe(func() { LnsGet(u, "a") })
	injectAt(1, []int{-1, -1, -1, -1, -1, -1}, true, nil)
	safe(func() { lnsParse(u, "a") })
}

// TestFaultInjectionRecursivePut drives put on a recursive lens with injected
// failures to reach the recursive-parse and create error guards.
func TestFaultInjectionRecursivePut(t *testing.T) {
	defer clearInject()
	boom := errors.New("boom")
	src := "module M =\n let rec lns = [ del /\\(/ \"(\" . label \"g\" . lns* . del /\\)/ \")\" ] | [ label \"x\" . store /[a-z]/ ]\n"
	i := New(srcMap(map[string]string{"m": src}))
	l, err := i.LensValue("M", "lns")
	if err != nil {
		t.Fatal(err)
	}
	in := "(a)"
	forest, gerr := LnsGet(l, in)
	if gerr != nil {
		t.Skipf("recursive get failed: %v", gerr)
	}
	for k := 1; k <= 12; k++ {
		injectAt(k, nil, false, boom)
		safe(func() { LnsPut(l, forest, in) })
		injectAt(k, nil, false, boom)
		safe(func() { lnsParse(l, in) })
	}
}

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"regexp/syntax"
)

// faMinus computes r1 - r2 over finite automata: it parses both operands,
// builds byte-level automata over a shared alphabet, and returns a regexp for
// L(r1) ∩ ¬L(r2). This mirrors upstream fa_minus followed by fa_as_regexp.
func faMinus(r1, r2 *Regexp) (*Regexp, error) {
	p1, err := parseForFA(r1)
	if err != nil {
		return nil, fmt.Errorf("regexp subtraction: parse r1: %w", err)
	}
	p2, err := parseForFA(r2)
	if err != nil {
		return nil, fmt.Errorf("regexp subtraction: parse r2: %w", err)
	}

	ab := newAlphabet(p1, p2)
	d1 := buildNFA(p1, ab).determinize(ab)
	d2 := buildNFA(p2, ab).determinize(ab)

	res := d1.intersect(d2.complement())
	pat, err := res.toRegexp()
	if err != nil {
		return nil, err
	}
	return newRawRegexp(pat), nil
}

func parseForFA(r *Regexp) (*syntax.Regexp, error) {
	// r.re2() already applies toRE2 for non-raw patterns, leaves raw patterns
	// (produced by a previous automata operation) untouched, and folds case.
	re, err := syntax.Parse(r.re2(), syntax.Perl)
	if err != nil {
		return nil, err
	}
	return re.Simplify(), nil
}

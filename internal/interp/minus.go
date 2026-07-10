// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

// regexpMinus computes the difference r1 - r2 as a regexp. Augeas implements
// this over finite automata (fa_minus + fa_as_regexp); see minus_fa.go for the
// pure-Go automata port.
func regexpMinus(r1, r2 *Regexp) (*Regexp, error) {
	return faMinus(r1, r2)
}

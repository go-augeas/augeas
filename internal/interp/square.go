// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

// squareLimit bounds the number of delimiter words enumerated when computing a
// square lens's precise ctype, matching upstream's fixed limit of 10 in
// square_precise_type (lens.c).
const squareLimit = 10

// squarePreciseType reproduces upstream square_precise_type: it enumerates the
// (finite) language of the left delimiter regexp l1 and, for each word w,
// builds w . body . w, unioning the results. This yields the exact balanced
// language of the square rather than the loose l1 . body . l3 concatenation, so
// that unbalanced strings (e.g. a bare `"value` where only the opening quote is
// present) are correctly rejected at ctype level — which is what disambiguates
// quoted/unquoted union branches the way the C reference does.
//
// When the delimiter language is infinite or has more than squareLimit words
// (e.g. arbitrary XML/section tag names), enumeration fails and the caller
// keeps the loose concatenation ctype, exactly as upstream does.
func squarePreciseType(l1, body *Regexp) *Regexp {
	if l1 == nil {
		return nil
	}
	words, ok := enumerate(l1, squareLimit)
	if !ok || len(words) == 0 {
		return nil
	}
	alts := make([]*Regexp, 0, len(words))
	for _, w := range words {
		lit := makeRegexpLiteral(w)
		alts = append(alts, regexpConcatN([]*Regexp{lit, body, lit}))
	}
	return regexpUnionN(alts)
}

// enumerate returns the words of r's language, or ok=false if the language is
// infinite or contains more than limit words. It walks the DFA of r over the
// shared byte alphabet, expanding each symbol interval into its concrete bytes
// so that a character class such as /[a-z]/ is correctly counted as 26 words
// (and thus exceeds a small limit), mirroring fa_enumerate.
func enumerate(r *Regexp, limit int) (words []string, ok bool) {
	re, err := parseForFA(r)
	if err != nil {
		return nil, false
	}
	ab := newAlphabet(re)
	d := buildNFA(re, ab).determinize(ab)
	nsym := ab.nsym()

	onPath := make([]bool, len(d.trans))
	// dfs appends every accepted word reachable from state; it returns false to
	// abort the whole enumeration when a cycle (infinite language) is found or
	// the word count exceeds limit.
	var dfs func(state int, prefix []byte) bool
	dfs = func(state int, prefix []byte) bool {
		if onPath[state] {
			return false // cycle: infinite language
		}
		if d.accept[state] {
			words = append(words, string(prefix))
			if len(words) > limit {
				return false
			}
		}
		onPath[state] = true
		for sym := 0; sym < nsym; sym++ {
			nx := d.trans[state][sym]
			if nx < 0 {
				continue
			}
			lo, hi := ab.interval(sym)
			if hi > 255 {
				hi = 255
			}
			for b := lo; b <= hi; b++ {
				next := make([]byte, len(prefix)+1)
				copy(next, prefix)
				next[len(prefix)] = byte(b)
				if !dfs(nx, next) {
					onPath[state] = false
					return false
				}
			}
		}
		onPath[state] = false
		return true
	}
	if !dfs(d.start, nil) {
		return nil, false
	}
	return words, true
}

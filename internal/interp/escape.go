// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "strings"

// escapeChars and escapeNames mirror upstream Augeas internal.c: the C escape
// sequences understood inside string and regexp literals.
const escapeChars = "\a\b\t\n\v\f\r"
const escapeNames = "abtnvfr"

// unescape reverses the C-style escaping done in the .aug lexer. extra names
// the additional characters (besides the standard control escapes) that a
// backslash may escape; for string literals that is `"\` and for regexp
// literals it is `/\`. Any other backslash sequence is passed through verbatim
// (the backslash is retained), matching upstream unescape().
func unescape(s string, extra string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			n := s[i+1]
			if idx := strings.IndexByte(escapeNames, n); idx >= 0 {
				b.WriteByte(escapeChars[idx])
				i++
				continue
			}
			if strings.IndexByte(extra, n) >= 0 {
				b.WriteByte(n)
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// unescapeString processes a DQUOTED literal body (STR_ESCAPES = "\"\\").
func unescapeString(s string) string { return unescape(s, "\"\\") }

// unescapeRegexp processes a REGEXP literal body (RX_ESCAPES = "/\\").
func unescapeRegexp(s string) string { return unescape(s, "/\\") }

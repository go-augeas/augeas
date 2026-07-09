// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "strings"

// re2Meta are the characters that carry special meaning in RE2 outside a
// character class and therefore must keep a preceding backslash to be taken
// literally.
const re2Meta = `.+*?()|[]{}^$\`

// toRE2 translates an Augeas-dialect pattern (already unescaped by the lexer)
// into an equivalent RE2 pattern, preserving the capture-group structure
// one-for-one so that register numbering stays identical to upstream. The
// Augeas dialect used by the corpus is a POSIX-ish ERE without back-references
// or GNU operators; the main adjustments are dropping backslashes before
// characters RE2 does not accept as escapes.
func toRE2(p string) (string, error) {
	var b strings.Builder
	i := 0
	n := len(p)
	for i < n {
		c := p[i]
		switch c {
		case '\\':
			if i+1 >= n {
				b.WriteString(`\\`)
				i++
				continue
			}
			nx := p[i+1]
			if strings.IndexByte(re2Meta, nx) >= 0 {
				b.WriteByte('\\')
				b.WriteByte(nx)
			} else {
				// Augeas allows escaping arbitrary characters; RE2 rejects
				// unknown escapes, so emit the character literally.
				writeLiteral(&b, nx)
			}
			i += 2
		case '[':
			j, cls := translateClass(p, i)
			b.WriteString(cls)
			i = j
		case '*', '+', '?':
			// Collapse a run of stacked quantifiers into a single equivalent
			// one. Augeas's matcher accepts e.g. "x*+" (= (x*)+) which RE2
			// rejects as nested repetition; since we only need language
			// equality under leftmost-longest, (x*)+ = x*, (x?)+ = x*, etc.
			canZero, canMany := false, false
			for i < n && (p[i] == '*' || p[i] == '+' || p[i] == '?') {
				if p[i] == '*' || p[i] == '?' {
					canZero = true
				}
				if p[i] == '*' || p[i] == '+' {
					canMany = true
				}
				i++
			}
			switch {
			case canZero && canMany:
				b.WriteByte('*')
			case canZero:
				b.WriteByte('?')
			default:
				b.WriteByte('+')
			}
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String(), nil
}

// writeLiteral writes a single byte, escaping it if RE2 would treat it as meta.
func writeLiteral(b *strings.Builder, c byte) {
	if strings.IndexByte(re2Meta, c) >= 0 {
		b.WriteByte('\\')
	}
	b.WriteByte(c)
}

// translateClass copies a character class starting at p[start]=='[' and returns
// the index just past the closing ']' and the RE2 translation. Augeas follows
// GNU regex without RE_BACKSLASH_ESCAPE_IN_LISTS, so inside a bracket expression
// a backslash is an ordinary member and the first ']' (except in leading
// position) always closes the class. We re-escape members for RE2, where
// backslash *is* special. POSIX classes [:name:] are recognised and copied.
func translateClass(p string, start int) (int, string) {
	var b strings.Builder
	b.WriteByte('[')
	i := start + 1
	n := len(p)
	if i < n && p[i] == '^' {
		b.WriteByte('^')
		i++
	}
	// A ']' immediately here is a literal member.
	if i < n && p[i] == ']' {
		b.WriteString(`\]`)
		i++
	}
	for i < n {
		c := p[i]
		if c == ']' {
			b.WriteByte(']')
			i++
			return i, b.String()
		}
		if c == '[' && i+1 < n && p[i+1] == ':' {
			if j := posixClassEnd(p, i); j > 0 {
				b.WriteString(p[i:j])
				i = j
				continue
			}
			b.WriteByte('[')
			i++
			continue
		}
		if c == '\\' {
			// Backslash is an ordinary member in Augeas classes (GNU regex
			// without RE_BACKSLASH_ESCAPE_IN_LISTS). Escape it for RE2.
			b.WriteString(`\\`)
			i++
			continue
		}
		b.WriteByte(c)
		i++
	}
	// Unterminated class: close it defensively.
	b.WriteByte(']')
	return i, b.String()
}

// posixClassEnd reports the index just past a well-formed POSIX class token
// [:name:] starting at p[i]=='[', or 0 if the text there is not such a token.
func posixClassEnd(p string, i int) int {
	// p[i] == '[', p[i+1] == ':'
	j := i + 2
	if j < len(p) && p[j] == '^' {
		j++
	}
	start := j
	for j < len(p) && ((p[j] >= 'a' && p[j] <= 'z') || (p[j] >= 'A' && p[j] <= 'Z')) {
		j++
	}
	if j == start {
		return 0
	}
	if j+1 < len(p) && p[j] == ':' && p[j+1] == ']' {
		return j + 2
	}
	return 0
}

// expandNocase rewrites a pattern so it matches case-insensitively without a
// global flag, by duplicating letters into character classes. It mirrors the
// role of fa_expand_nocase for the rare mixed-case combinations in the corpus.
func expandNocase(p string) string {
	var b strings.Builder
	i := 0
	n := len(p)
	for i < n {
		c := p[i]
		switch {
		case c == '\\' && i+1 < n:
			b.WriteByte(c)
			b.WriteByte(p[i+1])
			i += 2
		case c == '[':
			j, cls := expandClassNocase(p, i)
			b.WriteString(cls)
			i = j
		case isLetter(c):
			b.WriteByte('[')
			b.WriteByte(swapLower(c))
			b.WriteByte(swapUpper(c))
			b.WriteByte(']')
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func expandClassNocase(p string, start int) (int, string) {
	// Find the end of the class first.
	i := start + 1
	n := len(p)
	if i < n && p[i] == '^' {
		i++
	}
	if i < n && p[i] == ']' {
		i++
	}
	bodyStart := start
	for i < n {
		if p[i] == ']' {
			i++
			break
		}
		i++
	}
	body := p[bodyStart:i] // includes [ and ]
	// Insert case-swapped duplicates before the closing ']'.
	inner := body[1 : len(body)-1]
	var extra strings.Builder
	for k := 0; k < len(inner); k++ {
		ch := inner[k]
		if isLetter(ch) {
			if k+2 < len(inner) && inner[k+1] == '-' && isLetter(inner[k+2]) {
				extra.WriteByte(swapCase(ch))
				extra.WriteByte('-')
				extra.WriteByte(swapCase(inner[k+2]))
				k += 2
			} else {
				extra.WriteByte(swapCase(ch))
			}
		}
	}
	ex := extra.String()
	if ex == "" {
		return i, "[" + inner + "]"
	}
	// A trailing bare '-' is a literal member; inserting more characters after
	// it would form an invalid range, so keep it last.
	if strings.HasSuffix(inner, "-") && !strings.HasSuffix(inner, `\-`) {
		return i, "[" + inner[:len(inner)-1] + ex + "-]"
	}
	return i, "[" + inner + ex + "]"
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func swapCase(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

func swapLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c - 'A' + 'a'
	}
	return c
}

func swapUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

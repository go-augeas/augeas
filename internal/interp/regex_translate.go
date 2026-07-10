// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"strings"
)

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
				// unknown escapes. nx is not an RE2 metacharacter here, so emit
				// it literally.
				b.WriteByte(nx)
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

// restrictRE2 rewrites an RE2 pattern so it can never match the reserved
// encoding bytes \x01-\x04 (ENC_EQ/ENC_SLASH and friends). This mirrors
// upstream restrict_regexp, which keeps key/value types from spanning the
// encoded-tree separators during put. It rewrites "." and negated classes to
// also exclude the reserved range.
func restrictRE2(p string) string {
	const excl = `\x01-\x04`
	var b strings.Builder
	i, n := 0, len(p)
	for i < n {
		c := p[i]
		switch c {
		case '\\':
			if i+1 < n {
				b.WriteByte(c)
				b.WriteByte(p[i+1])
				i += 2
			} else {
				b.WriteByte(c)
				i++
			}
		case '.':
			b.WriteString(`[^` + excl + "\n]")
			i++
		case '[':
			j := re2ClassEnd(p, i)
			cls := p[i:j]
			if len(cls) >= 2 && cls[1] == '^' {
				// Insert the exclusion right after "[^" (and after a leading
				// literal ']') so it cannot merge with a trailing bare '-' into
				// an invalid range.
				ins := 2
				if ins < len(cls)-1 && cls[ins] == ']' {
					ins++
				}
				b.WriteString(cls[:ins])
				b.WriteString(excl)
				b.WriteString(cls[ins:])
			} else {
				// Positive class: clip the reserved byte range out of every
				// member so the class can never match \x01-\x04.
				b.WriteString(clipReservedClass(cls))
			}
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// clipReservedClass rewrites a positive RE2 character class so it excludes the
// reserved bytes \x01-\x04, by decoding its member byte ranges, subtracting
// [1,4], and re-emitting. cls includes the surrounding brackets.
func clipReservedClass(cls string) string {
	body := cls[1 : len(cls)-1]
	ranges, posix := parseClassBody(body)
	var out []byteRange
	for _, r := range ranges {
		out = append(out, subtractReserved(r)...)
	}
	if len(out) == 0 && posix == "" {
		// Nothing left; use a class that matches nothing meaningful but stays
		// valid. An empty positive class is invalid in RE2, so emit a range that
		// cannot occur in practice.
		return `[\x00]`
	}
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(posix)
	for _, r := range out {
		if r.lo == r.hi {
			b.WriteString(classByte(r.lo))
		} else {
			b.WriteString(classByte(r.lo))
			b.WriteByte('-')
			b.WriteString(classByte(r.hi))
		}
	}
	b.WriteByte(']')
	return b.String()
}

type byteRange struct{ lo, hi int }

// parseClassBody decodes an RE2 positive-class body into byte ranges plus any
// POSIX class tokens (which are copied through verbatim).
func parseClassBody(s string) ([]byteRange, string) {
	var ranges []byteRange
	var posix strings.Builder
	i, n := 0, len(s)
	for i < n {
		if s[i] == '[' && i+1 < n && s[i+1] == ':' {
			if j := posixClassEnd(s, i); j > 0 {
				posix.WriteString(s[i:j])
				i = j
				continue
			}
		}
		lo, adv := decodeClassByte(s, i)
		i = adv
		if i+1 < n && s[i] == '-' && s[i+1] != ']' {
			hi, adv2 := decodeClassByte(s, i+1)
			i = adv2
			ranges = append(ranges, byteRange{lo, hi})
		} else {
			ranges = append(ranges, byteRange{lo, lo})
		}
	}
	return ranges, posix.String()
}

// decodeClassByte decodes one class member byte at s[i], returning its value and
// the next index.
func decodeClassByte(s string, i int) (int, int) {
	if s[i] != '\\' || i+1 >= len(s) {
		return int(s[i]), i + 1
	}
	c := s[i+1]
	switch c {
	case 'x':
		if i+3 < len(s) {
			v := hexVal(s[i+2])*16 + hexVal(s[i+3])
			return v, i + 4
		}
		return int('x'), i + 2
	case 'n':
		return '\n', i + 2
	case 't':
		return '\t', i + 2
	case 'r':
		return '\r', i + 2
	case 'f':
		return '\f', i + 2
	case 'v':
		return '\v', i + 2
	case 'a':
		return '\a', i + 2
	default:
		return int(c), i + 2
	}
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return 0
}

// subtractReserved removes [1,4] from a byte range.
func subtractReserved(r byteRange) []byteRange {
	const lo, hi = 1, 4
	if r.hi < lo || r.lo > hi {
		return []byteRange{r}
	}
	var out []byteRange
	if r.lo < lo {
		out = append(out, byteRange{r.lo, lo - 1})
	}
	if r.hi > hi {
		out = append(out, byteRange{hi + 1, r.hi})
	}
	return out
}

// classByte renders a byte for inclusion in an RE2 character class.
func classByte(b int) string {
	switch byte(b) {
	case ']', '\\', '^', '-':
		return "\\" + string(byte(b))
	}
	if b < 0x20 || b > 0x7e {
		return fmt.Sprintf("\\x%02x", b)
	}
	return string(byte(b))
}

// re2ClassEnd returns the index just past the ']' closing the RE2 class at
// p[start]=='['. RE2 class semantics: backslash escapes; a ']' right after '['
// or '[^' is a literal member.
func re2ClassEnd(p string, start int) int {
	i := start + 1
	n := len(p)
	if i < n && p[i] == '^' {
		i++
	}
	if i < n && p[i] == ']' {
		i++
	}
	for i < n {
		if p[i] == '\\' && i+1 < n {
			i += 2
			continue
		}
		if p[i] == ']' {
			return i + 1
		}
		i++
	}
	return n
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

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"regexp"
	"strings"
)

// Regexp is Augeas's own regular-expression value. Following upstream, a
// regexp is represented as a pattern *string* in the Augeas dialect (already
// unescaped) plus a nocase flag; larger regexps are built by string
// concatenation that keeps a one-to-one correspondence between the syntactic
// group structure of the lens tree and the capture groups of the compiled
// pattern. Matching is delegated to Go's RE2 engine in leftmost-longest
// (POSIX) mode, which reproduces the semantics of the GNU regex matcher that
// upstream configures.
type Regexp struct {
	pattern string
	nocase  bool
	// raw marks pattern as already being in RE2 syntax (produced by the
	// automata-based operations such as regexp subtraction), so it must not be
	// passed through toRE2 again.
	raw bool

	compiled *regexp.Regexp
	nsubv    int
	built    bool
}

func newRegexp(pattern string, nocase bool) *Regexp {
	return &Regexp{pattern: pattern, nocase: nocase}
}

func newRawRegexp(re2 string) *Regexp {
	return &Regexp{pattern: re2, raw: true}
}

// re2 returns the RE2-syntax form of this regexp with case folding applied.
func (r *Regexp) re2() string {
	s := r.pattern
	if !r.raw {
		s, _ = toRE2(s)
	}
	if r.nocase {
		s = "(?i:" + s + ")"
	}
	return s
}

// anyRaw reports whether any operand is raw or nocase-mixed, requiring the
// combination to be built directly in RE2 syntax.
func anyRaw(rs []*Regexp) bool {
	for _, r := range rs {
		if r.raw {
			return true
		}
	}
	return false
}

// makeRegexpLiteral turns a plain string into a regexp that matches it
// literally, escaping the metacharacters, mirroring make_regexp_literal.
func makeRegexpLiteral(text string) *Regexp {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\\' && i+1 < len(text) {
			b.WriteByte(c)
			i++
			b.WriteByte(text[i])
			continue
		}
		if strings.IndexByte(".|{}[]()+*?", c) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return newRegexp(b.String(), false)
}

func regexpConcatN(rs []*Regexp) *Regexp {
	present := present(rs)
	if len(present) == 0 {
		return nil
	}
	if anyRaw(present) {
		var b strings.Builder
		for _, r := range present {
			b.WriteByte('(')
			b.WriteString(r.re2())
			b.WriteByte(')')
		}
		return newRawRegexp(b.String())
	}
	nnocase := countNocase(present)
	mixed := nnocase > 0 && nnocase < len(present)
	var b strings.Builder
	for _, r := range present {
		b.WriteByte('(')
		if mixed && r.nocase {
			b.WriteString(expandNocase(r.pattern))
		} else {
			b.WriteString(r.pattern)
		}
		b.WriteByte(')')
	}
	return newRegexp(b.String(), nnocase == len(present))
}

func regexpUnionN(rs []*Regexp) *Regexp {
	present := present(rs)
	if len(present) == 0 {
		return nil
	}
	if anyRaw(present) {
		var b strings.Builder
		for i, r := range present {
			if i > 0 {
				b.WriteByte('|')
			}
			b.WriteByte('(')
			b.WriteString(r.re2())
			b.WriteByte(')')
		}
		return newRawRegexp(b.String())
	}
	nnocase := countNocase(present)
	mixed := nnocase > 0 && nnocase < len(present)
	var b strings.Builder
	for i, r := range present {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteByte('(')
		if mixed && r.nocase {
			b.WriteString(expandNocase(r.pattern))
		} else {
			b.WriteString(r.pattern)
		}
		b.WriteByte(')')
	}
	return newRegexp(b.String(), nnocase == len(present))
}

func regexpConcat(a, b *Regexp) *Regexp { return regexpConcatN([]*Regexp{a, b}) }
func regexpUnion(a, b *Regexp) *Regexp  { return regexpUnionN([]*Regexp{a, b}) }

// regexpIter builds (p){min,max}, (p)*, or (p)+, mirroring regexp_iter.
func regexpIter(r *Regexp, min, max int) *Regexp {
	if r == nil {
		return nil
	}
	if r.raw || r.nocase {
		p := r.re2()
		var s string
		switch {
		case (min == 0 || min == 1) && max == -1:
			q := byte('*')
			if min == 1 {
				q = '+'
			}
			s = fmt.Sprintf("(%s)%c", p, q)
		case min == max:
			s = fmt.Sprintf("(%s){%d}", p, min)
		default:
			s = fmt.Sprintf("(%s){%d,%d}", p, min, max)
		}
		return newRawRegexp(s)
	}
	p := r.pattern
	var s string
	switch {
	case (min == 0 || min == 1) && max == -1:
		q := byte('*')
		if min == 1 {
			q = '+'
		}
		s = fmt.Sprintf("(%s)%c", p, q)
	case min == max:
		s = fmt.Sprintf("(%s){%d}", p, min)
	default:
		s = fmt.Sprintf("(%s){%d,%d}", p, min, max)
	}
	return newRegexp(s, r.nocase)
}

func regexpMaybe(r *Regexp) *Regexp {
	if r == nil {
		return nil
	}
	if r.raw || r.nocase {
		return newRawRegexp(fmt.Sprintf("(%s)?", r.re2()))
	}
	return newRegexp(fmt.Sprintf("(%s)?", r.pattern), r.nocase)
}

func regexpMakeEmpty() *Regexp { return newRegexp("()", false) }

func present(rs []*Regexp) []*Regexp {
	out := rs[:0:0]
	for _, r := range rs {
		if r != nil {
			out = append(out, r)
		}
	}
	return out
}

func countNocase(rs []*Regexp) int {
	n := 0
	for _, r := range rs {
		if r.nocase {
			n++
		}
	}
	return n
}

// build compiles the RE2 form of the pattern lazily.
func (r *Regexp) build() error {
	if r.built {
		if r.compiled == nil {
			return fmt.Errorf("regexp: cannot compile /%s/", r.pattern)
		}
		return nil
	}
	r.built = true
	re2 := r.pattern
	if !r.raw {
		var err error
		re2, err = toRE2(r.pattern)
		if err != nil {
			return err
		}
	}
	prefix := "^(?:"
	if r.nocase {
		prefix = "(?i)^(?:"
	}
	c, err := regexp.Compile(prefix + re2 + ")")
	if err != nil {
		return fmt.Errorf("regexp: /%s/ -> %q: %w", r.pattern, re2, err)
	}
	c.Longest()
	r.compiled = c
	r.nsubv = c.NumSubexp()
	return nil
}

func (r *Regexp) nsub() int {
	if err := r.build(); err != nil {
		return 0
	}
	return r.nsubv
}

// match returns the register array (start,end pairs per group, -1 if unmatched)
// for the longest match of r anchored at start within text[:end], and whether
// it matched. Offsets are absolute into text. Group 0 is the whole match.
func (r *Regexp) match(text string, start, end int) ([]int, bool, error) {
	if err := r.build(); err != nil {
		return nil, false, err
	}
	sub := text[start:end]
	loc := r.compiled.FindStringSubmatchIndex(sub)
	if loc == nil {
		return nil, false, nil
	}
	regs := make([]int, len(loc))
	for i, v := range loc {
		if v < 0 {
			regs[i] = -1
		} else {
			regs[i] = v + start
		}
	}
	return regs, true, nil
}

// String renders the pattern for diagnostics.
func (r *Regexp) String() string {
	if r == nil {
		return "<nil>"
	}
	s := "/" + r.pattern + "/"
	if r.nocase {
		s += "i"
	}
	return s
}

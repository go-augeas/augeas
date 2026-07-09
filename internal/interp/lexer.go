// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "fmt"

type tokKind int

const (
	tEOF tokKind = iota
	tModule
	tLet
	tLetRec
	tString // keyword "string"
	tRegexp // keyword "regexp"
	tLens   // keyword "lens"
	tIn
	tAutoload
	tTest
	tGet
	tPut
	tAfter
	tArrow
	tUIdent
	tLIdent
	tQIdent
	tDQuoted
	tRegexpLit
	tPipe
	tStar
	tQuestion
	tPlus
	tLParen
	tRParen
	tEq
	tColon
	tSemi
	tDot
	tLBracket
	tRBracket
	tLBrace
	tRBrace
	tMinus
)

type token struct {
	kind   tokKind
	sval   string
	nocase bool
	line   int
}

type lexer struct {
	src  string
	pos  int
	line int
}

func newLexer(src string) *lexer { return &lexer{src: src, line: 1} }

func (l *lexer) errorf(format string, a ...any) error {
	return fmt.Errorf("line %d: %s", l.line, fmt.Sprintf(format, a...))
}

var keywords = map[string]tokKind{
	"module":   tModule,
	"let":      tLet,
	"string":   tString,
	"regexp":   tRegexp,
	"lens":     tLens,
	"in":       tIn,
	"autoload": tAutoload,
	"test":     tTest,
	"get":      tGet,
	"put":      tPut,
	"after":    tAfter,
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// tokens lexes the whole source into a slice of tokens ending with tEOF.
func (l *lexer) tokens() ([]token, error) {
	var out []token
	for {
		t, err := l.next()
		if err != nil {
			return nil, err
		}
		out = append(out, t)
		if t.kind == tEOF {
			return out, nil
		}
	}
}

func (l *lexer) skipSpaceAndComments() error {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\n' {
			l.line++
			l.pos++
			continue
		}
		if isSpace(c) {
			l.pos++
			continue
		}
		if c == '(' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			if err := l.skipComment(); err != nil {
				return err
			}
			continue
		}
		return nil
	}
	return nil
}

func (l *lexer) skipComment() error {
	depth := 0
	for l.pos < len(l.src) {
		if l.src[l.pos] == '(' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '*' {
			depth++
			l.pos += 2
			continue
		}
		if l.src[l.pos] == '*' && l.pos+1 < len(l.src) && l.src[l.pos+1] == ')' {
			depth--
			l.pos += 2
			if depth == 0 {
				return nil
			}
			continue
		}
		if l.src[l.pos] == '\n' {
			l.line++
		}
		l.pos++
	}
	return l.errorf("unterminated comment")
}

func (l *lexer) next() (token, error) {
	if err := l.skipSpaceAndComments(); err != nil {
		return token{}, err
	}
	if l.pos >= len(l.src) {
		return token{kind: tEOF, line: l.line}, nil
	}
	c := l.src[l.pos]
	line := l.line
	// String literal
	if c == '"' {
		return l.lexQuoted(line)
	}
	// Regexp literal
	if c == '/' {
		return l.lexRegexp(line)
	}
	// Arrow
	if c == '-' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '>' {
		l.pos += 2
		return token{kind: tArrow, line: line}, nil
	}
	// Identifiers / keywords
	if isIdentStart(c) {
		return l.lexIdent(line)
	}
	// Single-char operators
	l.pos++
	switch c {
	case '|':
		return token{kind: tPipe, line: line}, nil
	case '*':
		return token{kind: tStar, line: line}, nil
	case '?':
		return token{kind: tQuestion, line: line}, nil
	case '+':
		return token{kind: tPlus, line: line}, nil
	case '(':
		return token{kind: tLParen, line: line}, nil
	case ')':
		return token{kind: tRParen, line: line}, nil
	case '=':
		return token{kind: tEq, line: line}, nil
	case ':':
		return token{kind: tColon, line: line}, nil
	case ';':
		return token{kind: tSemi, line: line}, nil
	case '.':
		return token{kind: tDot, line: line}, nil
	case '[':
		return token{kind: tLBracket, line: line}, nil
	case ']':
		return token{kind: tRBracket, line: line}, nil
	case '{':
		return token{kind: tLBrace, line: line}, nil
	case '}':
		return token{kind: tRBrace, line: line}, nil
	case '-':
		return token{kind: tMinus, line: line}, nil
	}
	return token{}, l.errorf("unexpected character %q", string(c))
}

func (l *lexer) lexQuoted(line int) (token, error) {
	l.pos++ // opening "
	start := l.pos
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' && l.pos+1 < len(l.src) {
			if l.src[l.pos+1] == '\n' {
				l.line++
			}
			l.pos += 2
			continue
		}
		if c == '"' {
			body := l.src[start:l.pos]
			l.pos++
			return token{kind: tDQuoted, sval: unescapeString(body), line: line}, nil
		}
		if c == '\n' {
			l.line++
		}
		l.pos++
	}
	return token{}, l.errorf("unterminated string literal")
}

func (l *lexer) lexRegexp(line int) (token, error) {
	l.pos++ // opening /
	start := l.pos
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		if c == '\\' && l.pos+1 < len(l.src) {
			if l.src[l.pos+1] == '\n' {
				l.line++
			}
			l.pos += 2
			continue
		}
		if c == '/' {
			body := l.src[start:l.pos]
			l.pos++
			nocase := false
			if l.pos < len(l.src) && l.src[l.pos] == 'i' {
				nocase = true
				l.pos++
			}
			return token{kind: tRegexpLit, sval: unescapeRegexp(body), nocase: nocase, line: line}, nil
		}
		if c == '\n' {
			l.line++
		}
		l.pos++
	}
	return token{}, l.errorf("unterminated regexp literal")
}

func (l *lexer) lexIdent(line int) (token, error) {
	start := l.pos
	upper := l.src[l.pos] >= 'A' && l.src[l.pos] <= 'Z'
	for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
		l.pos++
	}
	word := l.src[start:l.pos]
	if upper {
		// Possible qualified identifier UID.LID
		if l.pos+1 < len(l.src) && l.src[l.pos] == '.' && isIdentStartLower(l.src[l.pos+1]) {
			l.pos++ // dot
			lstart := l.pos
			for l.pos < len(l.src) && isIdentPart(l.src[l.pos]) {
				l.pos++
			}
			return token{kind: tQIdent, sval: word + "." + l.src[lstart:l.pos], line: line}, nil
		}
		return token{kind: tUIdent, sval: word, line: line}, nil
	}
	// let / let rec
	if word == "let" {
		if l.tryLetRec() {
			return token{kind: tLetRec, line: line}, nil
		}
		return token{kind: tLet, line: line}, nil
	}
	if k, ok := keywords[word]; ok {
		return token{kind: k, line: line}, nil
	}
	return token{kind: tLIdent, sval: word, line: line}, nil
}

// tryLetRec detects "let[ \t]+rec" followed by whitespace and consumes "rec".
func (l *lexer) tryLetRec() bool {
	p := l.pos
	sawSpace := false
	for p < len(l.src) && (l.src[p] == ' ' || l.src[p] == '\t') {
		p++
		sawSpace = true
	}
	if !sawSpace {
		return false
	}
	if p+3 <= len(l.src) && l.src[p:p+3] == "rec" {
		after := p + 3
		if after >= len(l.src) || isSpace(l.src[after]) {
			l.pos = after
			return true
		}
	}
	return false
}

func isIdentStartLower(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') }

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// This file implements the Augeas-subset path language. The supported grammar
// is documented in the README; the parser below turns a path string into a list
// of location steps, and the evaluator walks the tree applying them.

// axis selects which nodes a step considers relative to a context node.
type axis int

const (
	axisChild      axis = iota // name test against direct children
	axisDescendant             // name test against all descendants
	axisSelf                   // the context node itself (".")
	axisParent                 // the context node's parent ("..")
)

// predKind classifies a predicate inside [...].
type predKind int

const (
	predPos    predKind = iota // [n]
	predLast                   // [last()]
	predExists                 // [subpath]
	predEqual                  // [subpath = 'value']
	predMatch                  // [subpath =~ 'regexp']
)

// predicate is a single [...] filter applied to a step's candidate nodes.
type predicate struct {
	kind  predKind
	n     int            // for predPos
	path  string         // subpath for predExists/predEqual/predMatch ("." allowed)
	value string         // literal for predEqual
	re    *regexp.Regexp // compiled regexp for predMatch
}

// step is one location step: an axis, a name test and zero or more predicates.
type step struct {
	ax    axis
	name  string // label to match, or "*"
	preds []predicate
}

// splitTop splits s on sep, ignoring separators that fall inside [...] brackets
// or single quotes.
func splitTop(s string, sep byte) []string {
	var out []string
	depth, quoted, start := 0, false, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'':
			quoted = !quoted
		case quoted:
			// inside a quoted literal: ignore structure
		case c == '[':
			depth++
		case c == ']':
			depth--
		case c == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// parsePredicate parses the text inside a single pair of brackets.
func parsePredicate(body string) (predicate, error) {
	b := strings.TrimSpace(body)
	if b == "" {
		return predicate{}, fmt.Errorf("empty predicate")
	}
	if b == "last()" {
		return predicate{kind: predLast}, nil
	}
	if n, err := strconv.Atoi(b); err == nil {
		if n < 1 {
			return predicate{}, fmt.Errorf("position %d out of range", n)
		}
		return predicate{kind: predPos, n: n}, nil
	}
	if lhs, rhs, ok := splitOp(b, "=~"); ok {
		re, err := regexp.Compile(rhs)
		if err != nil {
			return predicate{}, fmt.Errorf("bad regexp %q: %w", rhs, err)
		}
		return predicate{kind: predMatch, path: lhs, re: re}, nil
	}
	if lhs, rhs, ok := splitOp(b, "="); ok {
		return predicate{kind: predEqual, path: lhs, value: rhs}, nil
	}
	return predicate{kind: predExists, path: b}, nil
}

// splitOp splits b at the first occurrence of op, trimming whitespace and, on
// the right-hand side, a single pair of surrounding single quotes. It reports
// whether op was found.
func splitOp(b, op string) (lhs, rhs string, ok bool) {
	i := strings.Index(b, op)
	if i < 0 {
		return "", "", false
	}
	lhs = strings.TrimSpace(b[:i])
	rhs = strings.TrimSpace(b[i+len(op):])
	if len(rhs) >= 2 && rhs[0] == '\'' && rhs[len(rhs)-1] == '\'' {
		rhs = rhs[1 : len(rhs)-1]
	}
	return lhs, rhs, true
}

// parseStep parses a single path segment (name plus predicates) with the given
// axis.
func parseStep(seg string, ax axis) (step, error) {
	name := seg
	var preds []predicate
	if i := strings.IndexByte(seg, '['); i >= 0 {
		name = seg[:i]
		rest := seg[i:]
		for len(rest) > 0 {
			if rest[0] != '[' {
				return step{}, fmt.Errorf("malformed predicate in %q", seg)
			}
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return step{}, fmt.Errorf("unterminated predicate in %q", seg)
			}
			p, err := parsePredicate(rest[1:end])
			if err != nil {
				return step{}, err
			}
			preds = append(preds, p)
			rest = rest[end+1:]
		}
	}
	if name == "" {
		return step{}, fmt.Errorf("empty name test in %q", seg)
	}
	return step{ax: ax, name: name, preds: preds}, nil
}

// parseSteps turns a slash-separated path body (no leading slash, no variable
// prefix) into a list of steps. An empty segment marks the descendant axis for
// the following step ("//").
func parseSteps(body string) ([]step, error) {
	if body == "" {
		return nil, nil
	}
	segs := splitTop(body, '/')
	var steps []step
	descendant := false
	for _, seg := range segs {
		if seg == "" {
			descendant = true
			continue
		}
		ax := axisChild
		switch {
		case descendant:
			ax = axisDescendant
		case seg == ".":
			steps = append(steps, step{ax: axisSelf, name: "."})
			continue
		case seg == "..":
			steps = append(steps, step{ax: axisParent, name: ".."})
			continue
		}
		descendant = false
		st, err := parseStep(seg, ax)
		if err != nil {
			return nil, err
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// descendants appends all descendants of n (excluding n) to dst.
func descendants(n *Node, dst []*Node) []*Node {
	for _, c := range n.Children {
		dst = append(dst, c)
		dst = descendants(c, dst)
	}
	return dst
}

// nameMatch reports whether label satisfies the step's name test.
func nameMatch(name, label string) bool {
	return name == "*" || name == label
}

// candidates returns the nodes selected by the step's axis and name test from
// context node ctx (before predicates).
func candidates(ctx *Node, s step) []*Node {
	switch s.ax {
	case axisSelf:
		return []*Node{ctx}
	case axisParent:
		if ctx.Parent == nil {
			return nil
		}
		return []*Node{ctx.Parent}
	case axisDescendant:
		var out []*Node
		for _, d := range descendants(ctx, nil) {
			if nameMatch(s.name, d.Label) {
				out = append(out, d)
			}
		}
		return out
	default: // axisChild
		var out []*Node
		for _, c := range ctx.Children {
			if nameMatch(s.name, c.Label) {
				out = append(out, c)
			}
		}
		return out
	}
}

// predHolds reports whether predicate p holds for node n at 1-based position pos
// within a candidate set of the given size.
func (a *Augeas) predHolds(n *Node, p predicate, pos, size int) bool {
	switch p.kind {
	case predPos:
		return pos == p.n
	case predLast:
		return pos == size
	}
	// Subpath predicates resolve a simple relative path (labels or "*",
	// slash-separated, or ".") against n. Nested predicates are not part of the
	// subset, so this resolver has no error case.
	subs := resolveSub(n, p.path)
	switch p.kind {
	case predExists:
		return len(subs) > 0
	case predEqual:
		for _, s := range subs {
			if s.Value != nil && *s.Value == p.value {
				return true
			}
		}
		return false
	default: // predMatch
		for _, s := range subs {
			if s.Value != nil && p.re.MatchString(*s.Value) {
				return true
			}
		}
		return false
	}
}

// applyPreds narrows cands by applying each predicate left to right.
func (a *Augeas) applyPreds(cands []*Node, preds []predicate) []*Node {
	for _, p := range preds {
		size := len(cands)
		var next []*Node
		for i, n := range cands {
			if a.predHolds(n, p, i+1, size) {
				next = append(next, n)
			}
		}
		cands = next
	}
	return cands
}

// resolveSub resolves a simple relative path (labels or "*", slash-separated,
// or ".") against node, returning the matching descendants. It supports no
// predicates and cannot fail, which keeps predicate evaluation total.
func resolveSub(node *Node, path string) []*Node {
	if path == "." {
		return []*Node{node}
	}
	ctx := []*Node{node}
	for _, seg := range splitTop(path, '/') {
		var next []*Node
		for _, c := range ctx {
			for _, ch := range c.Children {
				if nameMatch(seg, ch.Label) {
					next = append(next, ch)
				}
			}
		}
		ctx = next
	}
	return ctx
}

// evalStepsCtx evaluates steps from ctx, applying predicates.
func (a *Augeas) evalStepsCtx(ctx []*Node, steps []step) []*Node {
	for _, s := range steps {
		var out []*Node
		for _, c := range ctx {
			out = append(out, a.applyPreds(candidates(c, s), s.preds)...)
		}
		ctx = dedup(out)
	}
	return ctx
}

// dedup removes duplicate node pointers while preserving order.
func dedup(nodes []*Node) []*Node {
	seen := make(map[*Node]bool, len(nodes))
	var out []*Node
	for _, n := range nodes {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

// eval resolves a path expression against the tree, returning the matching
// nodes in document order. Union ("|") sub-expressions are evaluated in turn.
func (a *Augeas) eval(path string) ([]*Node, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}
	var all []*Node
	for _, expr := range splitTop(path, '|') {
		nodes, err := a.evalOne(strings.TrimSpace(expr))
		if err != nil {
			return nil, err
		}
		all = append(all, nodes...)
	}
	return dedup(all), nil
}

// evalOne resolves a single (non-union) path expression.
func (a *Augeas) evalOne(expr string) ([]*Node, error) {
	var ctx []*Node
	body := expr
	switch {
	case strings.HasPrefix(expr, "$"):
		name := expr[1:]
		if i := strings.IndexByte(name, '/'); i >= 0 {
			body = name[i+1:]
			name = name[:i]
		} else {
			body = ""
		}
		nodes, ok := a.vars[name]
		if !ok {
			return nil, fmt.Errorf("undefined variable $%s", name)
		}
		ctx = nodes
	case strings.HasPrefix(expr, "/"):
		body = strings.TrimPrefix(expr, "/")
		ctx = []*Node{a.root}
	default:
		ctx = []*Node{a.root}
	}
	steps, err := parseSteps(body)
	if err != nil {
		return nil, err
	}
	return a.evalStepsCtx(ctx, steps), nil
}

// buildPath returns the absolute path of n from the root.
func (a *Augeas) buildPath(n *Node) string {
	if n == a.root || n.Parent == nil {
		return "/"
	}
	var parts []string
	for cur := n; cur != a.root && cur.Parent != nil; cur = cur.Parent {
		parts = append([]string{cur.segment()}, parts...)
	}
	return "/" + strings.Join(parts, "/")
}

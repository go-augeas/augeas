// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"errors"
	"fmt"
	"strings"
)

// Augeas holds a configuration tree and the editing state (variables, the
// filesystem seam and the last error). Create one with [New].
type Augeas struct {
	root      *Node
	vars      map[string][]*Node
	fs        FileSystem
	lastError error
	eng       *Engine
}

// New returns an empty Augeas tree backed by the real filesystem.
func New() *Augeas {
	return &Augeas{
		root: &Node{Label: "/"},
		vars: map[string][]*Node{},
		fs:   osFS{},
	}
}

// Root returns the tree root. It is exposed for lens and test use.
func (a *Augeas) Root() *Node { return a.root }

// SetFileSystem replaces the filesystem seam used by Load and Save. Passing nil
// is a no-op so callers cannot accidentally disable I/O.
func (a *Augeas) SetFileSystem(fs FileSystem) {
	if fs != nil {
		a.fs = fs
	}
}

// Error returns the last error recorded by an operation, or nil.
func (a *Augeas) Error() error { return a.lastError }

// fail records err as the last error and returns it.
func (a *Augeas) fail(err error) error {
	a.lastError = err
	return err
}

// Get returns the value of the single node matching path. The boolean reports
// whether exactly one node matched; a valueless node yields ("", true). If the
// path is malformed or matches more than one node the last error is set.
func (a *Augeas) Get(path string) (string, bool) {
	nodes, err := a.eval(path)
	if err != nil {
		a.lastError = err
		return "", false
	}
	switch len(nodes) {
	case 0:
		return "", false
	case 1:
		if nodes[0].Value == nil {
			return "", true
		}
		return *nodes[0].Value, true
	default:
		a.lastError = fmt.Errorf("path %q matches %d nodes", path, len(nodes))
		return "", false
	}
}

// Exists reports whether at least one node matches path. A malformed path sets
// the last error and yields false.
func (a *Augeas) Exists(path string) bool {
	nodes, err := a.eval(path)
	if err != nil {
		a.lastError = err
		return false
	}
	return len(nodes) > 0
}

// Set sets the value of the node matching path. The path must match exactly one
// node; if it matches none, the node (and any missing ancestors) is created.
func (a *Augeas) Set(path, value string) error {
	nodes, err := a.eval(path)
	if err != nil {
		return a.fail(err)
	}
	switch len(nodes) {
	case 0:
		n, err := a.createPath(path)
		if err != nil {
			return a.fail(err)
		}
		n.SetValue(value)
	case 1:
		nodes[0].SetValue(value)
	default:
		return a.fail(fmt.Errorf("path %q matches %d nodes", path, len(nodes)))
	}
	return nil
}

// SetMultiple sets value on every node matching the sub path relative to each
// node matching base, creating the sub node where absent. It returns the number
// of nodes set.
func (a *Augeas) SetMultiple(base, sub, value string) (int, error) {
	bases, err := a.eval(base)
	if err != nil {
		return 0, a.fail(err)
	}
	steps, err := parseSteps(sub)
	if err != nil {
		return 0, a.fail(err)
	}
	count := 0
	for _, b := range bases {
		matches := a.evalStepsCtx([]*Node{b}, steps)
		if len(matches) == 0 {
			n, err := a.createFrom(b, sub)
			if err != nil {
				return count, a.fail(err)
			}
			matches = []*Node{n}
		}
		for _, m := range matches {
			m.SetValue(value)
			count++
		}
	}
	return count, nil
}

// Insert inserts a new, valueless sibling labelled label next to the single
// node matching path, before it when before is true, otherwise after it.
func (a *Augeas) Insert(path, label string, before bool) error {
	if label == "" || strings.ContainsAny(label, "/[]*") {
		return a.fail(fmt.Errorf("invalid insert label %q", label))
	}
	nodes, err := a.eval(path)
	if err != nil {
		return a.fail(err)
	}
	if len(nodes) != 1 {
		return a.fail(fmt.Errorf("insert path %q matches %d nodes", path, len(nodes)))
	}
	n := nodes[0]
	if n.Parent == nil {
		return a.fail(errors.New("cannot insert next to the root"))
	}
	p := n.Parent
	idx := 0
	for i, c := range p.Children {
		if c == n {
			idx = i
			break
		}
	}
	if !before {
		idx++
	}
	nn := newNode(label)
	nn.Parent = p
	p.Children = append(p.Children, nil)
	copy(p.Children[idx+1:], p.Children[idx:])
	p.Children[idx] = nn
	return nil
}

// Remove deletes every node matching path together with its subtree, and
// returns the number of nodes removed. The root is never removed.
func (a *Augeas) Remove(path string) int {
	nodes, err := a.eval(path)
	if err != nil {
		a.lastError = err
		return 0
	}
	count := 0
	for _, n := range nodes {
		if n == a.root || n.Parent == nil {
			continue
		}
		if n.Parent.removeChild(n) {
			count++
		}
	}
	return count
}

// Move moves the single node matching src (with its subtree) onto dst. dst may
// match one existing node (overwritten) or none (created). dst must not be a
// descendant of src.
func (a *Augeas) Move(src, dst string) error {
	srcs, err := a.eval(src)
	if err != nil {
		return a.fail(err)
	}
	if len(srcs) != 1 {
		return a.fail(fmt.Errorf("move source %q matches %d nodes", src, len(srcs)))
	}
	s := srcs[0]
	if s == a.root || s.Parent == nil {
		return a.fail(errors.New("cannot move the root"))
	}
	dsts, err := a.eval(dst)
	if err != nil {
		return a.fail(err)
	}
	if len(dsts) > 1 {
		return a.fail(fmt.Errorf("move destination %q matches %d nodes", dst, len(dsts)))
	}
	var d *Node
	if len(dsts) == 1 {
		d = dsts[0]
		if d == s || isDescendant(d, s) {
			return a.fail(errors.New("move destination is inside the source"))
		}
	} else {
		d, err = a.createPath(dst)
		if err != nil {
			return a.fail(err)
		}
	}
	s.Parent.removeChild(s)
	d.Value = s.Value
	d.Children = s.Children
	for _, c := range d.Children {
		c.Parent = d
	}
	return nil
}

// isDescendant reports whether n is inside the subtree rooted at anc.
func isDescendant(n, anc *Node) bool {
	for cur := n.Parent; cur != nil; cur = cur.Parent {
		if cur == anc {
			return true
		}
	}
	return false
}

// Match returns the absolute paths of all nodes matching path, in document
// order. A malformed path sets the last error and yields nil.
func (a *Augeas) Match(path string) []string {
	nodes, err := a.eval(path)
	if err != nil {
		a.lastError = err
		return nil
	}
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, a.buildPath(n))
	}
	return out
}

// Label returns the label of the single node matching path.
func (a *Augeas) Label(path string) (string, bool) {
	nodes, err := a.eval(path)
	if err != nil {
		a.lastError = err
		return "", false
	}
	if len(nodes) != 1 {
		return "", false
	}
	return nodes[0].Label, true
}

// DefineVariable binds name to the node-set produced by expr; the variable can
// then be used as "$name" at the head of a path. It returns the number of nodes
// bound.
func (a *Augeas) DefineVariable(name, expr string) (int, error) {
	nodes, err := a.eval(expr)
	if err != nil {
		return 0, a.fail(err)
	}
	a.vars[name] = nodes
	return len(nodes), nil
}

// DefineNode binds name to the nodes matching expr. When expr matches nothing,
// a single node is created at expr with the given value. It returns the path of
// the (first) bound node and whether a node was created.
func (a *Augeas) DefineNode(name, expr, value string) (string, bool) {
	nodes, err := a.eval(expr)
	if err != nil {
		a.lastError = err
		return "", false
	}
	if len(nodes) > 0 {
		a.vars[name] = nodes
		return a.buildPath(nodes[0]), false
	}
	n, err := a.createPath(expr)
	if err != nil {
		a.lastError = err
		return "", false
	}
	n.SetValue(value)
	a.vars[name] = []*Node{n}
	return a.buildPath(n), true
}

// Span is not tracked by this engine: byte offsets are not retained when a lens
// parses text. It always returns ErrSpanUnsupported so callers can detect the
// gap explicitly rather than silently receiving zeroes.
type Span struct {
	Filename                                               string
	LabelStart, LabelEnd, ValueStart, ValueEnd, Start, End int
}

// ErrSpanUnsupported is returned by [Augeas.Span]; span tracking is a documented
// deferred feature.
var ErrSpanUnsupported = errors.New("augeas: span information is not tracked")

// Span always returns ErrSpanUnsupported (see the type documentation).
func (a *Augeas) Span(path string) (Span, error) {
	return Span{}, ErrSpanUnsupported
}

// createPath creates the node addressed by an absolute, plain (no predicate or
// wildcard) path, creating missing ancestors, and returns it.
func (a *Augeas) createPath(path string) (*Node, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("cannot create relative path %q", path)
	}
	return a.createFrom(a.root, strings.TrimPrefix(path, "/"))
}

// createFrom creates the node addressed by the plain slash path body relative to
// base, creating missing intermediate nodes, and returns the final node.
func (a *Augeas) createFrom(base *Node, body string) (*Node, error) {
	cur := base
	for _, seg := range splitTop(body, '/') {
		if seg == "" || strings.ContainsAny(seg, "[]*|") {
			return nil, fmt.Errorf("cannot create path segment %q", seg)
		}
		next := cur.firstChild(seg)
		if next == nil {
			next = newNode(seg)
			cur.appendChild(next)
		}
		cur = next
	}
	return cur, nil
}

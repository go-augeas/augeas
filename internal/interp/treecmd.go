// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import (
	"fmt"
	"strconv"
	"strings"
)

// This file implements the small tree-editing command language used by the
// `put` unit tests (set/rm/clear/insa/insb), operating on a forest under a
// synthetic root node.

type predKind int

const (
	predNone     predKind = iota
	predIndex             // [n]
	predLast              // [last()]
	predLastPlus          // [last()+1] — addresses a new node past the end
	predValueEq           // [. = 'v']
	predChild             // [child] or [child = 'v']
)

type pathSeg struct {
	label    string
	wildcard bool
	kind     predKind
	index    int
	val      string
	child    string
	childVal string
	childHas bool // whether child predicate checks value
}

func parsePath(p string) ([]pathSeg, error) {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil, nil
	}
	var segs []pathSeg
	for _, part := range splitPathParts(p) {
		seg := pathSeg{}
		label := part
		if i := strings.IndexByte(part, '['); i >= 0 {
			if !strings.HasSuffix(part, "]") {
				return nil, fmt.Errorf("bad path segment %q", part)
			}
			label = part[:i]
			pred := strings.TrimSpace(part[i+1 : len(part)-1])
			parsePredicate(&seg, pred)
		}
		if label == "*" {
			seg.wildcard = true
		}
		seg.label = label
		segs = append(segs, seg)
	}
	return segs, nil
}

// splitPathParts splits on '/' but not inside [...] predicates.
func splitPathParts(p string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '/':
			if depth == 0 {
				parts = append(parts, p[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, p[start:])
	return parts
}

func parsePredicate(seg *pathSeg, pred string) {
	switch {
	case pred == "last()":
		seg.kind = predLast
	case pred == "last()+1" || pred == "last() + 1":
		seg.kind = predLastPlus
	case strings.HasPrefix(pred, "."):
		// . = 'v'  /  . = "v"
		rest := strings.TrimSpace(strings.TrimPrefix(pred, "."))
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
		seg.kind = predValueEq
		seg.val = unquotePred(rest)
	default:
		if n, err := strconv.Atoi(pred); err == nil {
			seg.kind = predIndex
			seg.index = n
			return
		}
		// child existence or child = 'v'
		seg.kind = predChild
		if eq := strings.Index(pred, "="); eq >= 0 {
			seg.child = strings.TrimSpace(pred[:eq])
			seg.childVal = unquotePred(strings.TrimSpace(pred[eq+1:]))
			seg.childHas = true
		} else {
			seg.child = pred
		}
	}
}

func unquotePred(s string) string {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

func labelEq(t *Tree, label string) bool {
	if t.Label == nil {
		return label == ""
	}
	return *t.Label == label
}

func childValue(t *Tree, label string) (*string, bool) {
	for _, c := range t.Children {
		if labelEq(c, label) {
			return c.Value, true
		}
	}
	return nil, false
}

// findChildren returns the children of parent matching one path segment.
func findChildren(parent *Tree, seg pathSeg) []*Tree {
	var matched []*Tree
	for _, c := range parent.Children {
		if seg.wildcard || labelEq(c, seg.label) {
			matched = append(matched, c)
		}
	}
	switch seg.kind {
	case predIndex:
		if seg.index >= 1 && seg.index <= len(matched) {
			return matched[seg.index-1 : seg.index]
		}
		return nil
	case predLast:
		if len(matched) > 0 {
			return matched[len(matched)-1:]
		}
		return nil
	case predLastPlus:
		return nil // addresses a not-yet-existing node
	case predValueEq:
		var out []*Tree
		for _, c := range matched {
			if c.Value != nil && *c.Value == seg.val {
				out = append(out, c)
			}
		}
		return out
	case predChild:
		var out []*Tree
		for _, c := range matched {
			v, ok := childValue(c, seg.child)
			if !ok {
				continue
			}
			if seg.childHas {
				if v != nil && *v == seg.childVal {
					out = append(out, c)
				}
			} else {
				out = append(out, c)
			}
		}
		return out
	default:
		return matched
	}
}

func findNodes(root *Tree, segs []pathSeg) []*Tree {
	cur := []*Tree{root}
	for _, seg := range segs {
		var next []*Tree
		for _, n := range cur {
			next = append(next, findChildren(n, seg)...)
		}
		cur = next
	}
	return cur
}

// createPath creates (or returns) the single node addressed by segs, creating
// missing ancestors. It errors if a segment matches more than one node.
func createPathTree(root *Tree, segs []pathSeg) (*Tree, error) {
	cur := root
	for _, seg := range segs {
		matched := findChildren(cur, seg)
		switch len(matched) {
		case 1:
			cur = matched[0]
		case 0:
			label := seg.label
			n := &Tree{Label: strptr(label)}
			cur.Children = append(cur.Children, n)
			cur = n
		default:
			return nil, fmt.Errorf("path segment %q matches %d nodes", seg.label, len(matched))
		}
	}
	return cur, nil
}

func removeChild(parent, child *Tree) bool {
	for i, c := range parent.Children {
		if c == child {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			return true
		}
	}
	return false
}

// parentOf returns the parent of target within the tree rooted at root.
func parentOf(root, target *Tree) *Tree {
	for _, c := range root.Children {
		if c == target {
			return root
		}
		if p := parentOf(c, target); p != nil {
			return p
		}
	}
	return nil
}

func treeCmdSet(root *Tree, path, value string) error {
	segs, err := parsePath(path)
	if err != nil {
		return err
	}
	nodes := findNodes(root, segs)
	switch len(nodes) {
	case 0:
		n, err := createPathTree(root, segs)
		if err != nil {
			return err
		}
		n.Value = strptr(value)
	case 1:
		nodes[0].Value = strptr(value)
	default:
		return fmt.Errorf("set path %q matches %d nodes", path, len(nodes))
	}
	return nil
}

func treeCmdClear(root *Tree, path string) error {
	segs, err := parsePath(path)
	if err != nil {
		return err
	}
	nodes := findNodes(root, segs)
	if len(nodes) == 0 {
		n, err := createPathTree(root, segs)
		if err != nil {
			return err
		}
		n.Value = nil
		return nil
	}
	for _, n := range nodes {
		n.Value = nil
	}
	return nil
}

func treeCmdRm(root *Tree, path string) error {
	segs, err := parsePath(path)
	if err != nil {
		return err
	}
	for _, n := range findNodes(root, segs) {
		if p := parentOf(root, n); p != nil {
			removeChild(p, n)
		}
	}
	return nil
}

func treeCmdIns(root *Tree, label, path string, before bool) error {
	segs, err := parsePath(path)
	if err != nil {
		return err
	}
	nodes := findNodes(root, segs)
	if len(nodes) != 1 {
		return fmt.Errorf("ins path %q matches %d nodes", path, len(nodes))
	}
	target := nodes[0]
	if target == root {
		// Inserting next to "/" adds a new top-level node.
		nn := &Tree{Label: strptr(label)}
		if before {
			root.Children = append([]*Tree{nn}, root.Children...)
		} else {
			root.Children = append(root.Children, nn)
		}
		return nil
	}
	parent := parentOf(root, target)
	if parent == nil {
		return fmt.Errorf("cannot insert next to root")
	}
	idx := 0
	for i, c := range parent.Children {
		if c == target {
			idx = i
			break
		}
	}
	if !before {
		idx++
	}
	nn := &Tree{Label: strptr(label)}
	parent.Children = append(parent.Children, nil)
	copy(parent.Children[idx+1:], parent.Children[idx:])
	parent.Children[idx] = nn
	return nil
}

// registerTreeCmds adds the tree-editing natives used by put tests.
func registerTreeCmds(i *interp) {
	reg := func(name string, arity int, fn func(*interp, []Value) (Value, error)) {
		i.global.bind(name, &vNative{name: name, arity: arity, fn: fn})
	}
	withTree := func(v Value, f func(root *Tree) error) (Value, error) {
		tv, ok := v.(*vTree)
		if !ok {
			return nil, fmt.Errorf("tree command: last argument must be a tree")
		}
		root := &Tree{Children: tv.forest}
		if err := f(root); err != nil {
			return nil, err
		}
		return &vTree{forest: root.Children}, nil
	}
	str := func(v Value) (string, error) {
		s, ok := v.(vString)
		if !ok {
			return "", fmt.Errorf("expected string argument")
		}
		return string(s), nil
	}

	reg("set", 3, func(_ *interp, a []Value) (Value, error) {
		p, err := str(a[0])
		if err != nil {
			return nil, err
		}
		v, err := str(a[1])
		if err != nil {
			return nil, err
		}
		return withTree(a[2], func(root *Tree) error { return treeCmdSet(root, p, v) })
	})
	reg("clear", 2, func(_ *interp, a []Value) (Value, error) {
		p, err := str(a[0])
		if err != nil {
			return nil, err
		}
		return withTree(a[1], func(root *Tree) error { return treeCmdClear(root, p) })
	})
	reg("rm", 2, func(_ *interp, a []Value) (Value, error) {
		p, err := str(a[0])
		if err != nil {
			return nil, err
		}
		return withTree(a[1], func(root *Tree) error { return treeCmdRm(root, p) })
	})
	reg("insa", 3, func(_ *interp, a []Value) (Value, error) {
		lbl, err := str(a[0])
		if err != nil {
			return nil, err
		}
		p, err := str(a[1])
		if err != nil {
			return nil, err
		}
		return withTree(a[2], func(root *Tree) error { return treeCmdIns(root, lbl, p, false) })
	})
	reg("insb", 3, func(_ *interp, a []Value) (Value, error) {
		lbl, err := str(a[0])
		if err != nil {
			return nil, err
		}
		p, err := str(a[1])
		if err != nil {
			return nil, err
		}
		return withTree(a[2], func(root *Tree) error { return treeCmdIns(root, lbl, p, true) })
	})
}

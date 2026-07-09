// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import "strconv"

// Node is a single entry in the configuration tree. A node has a label, an
// optional value and an ordered list of children. Siblings may share a label;
// such siblings are distinguished by their 1-based position, matching Augeas'
// label[n] addressing.
type Node struct {
	Label    string
	Value    *string
	Children []*Node
	Parent   *Node
}

// newNode returns a detached node with the given label and no value.
func newNode(label string) *Node {
	return &Node{Label: label}
}

// SetValue sets the node's value.
func (n *Node) SetValue(v string) {
	n.Value = &v
}

// ClearValue removes the node's value, leaving it valueless.
func (n *Node) ClearValue() {
	n.Value = nil
}

// appendChild appends c to n's children and sets c.Parent.
func (n *Node) appendChild(c *Node) {
	c.Parent = n
	n.Children = append(n.Children, c)
}

// child returns the index-th (1-based) child whose label equals label, or nil.
func (n *Node) child(label string, index int) *Node {
	seen := 0
	for _, c := range n.Children {
		if c.Label == label {
			seen++
			if seen == index {
				return c
			}
		}
	}
	return nil
}

// firstChild returns the first child whose label equals label, or nil.
func (n *Node) firstChild(label string) *Node {
	return n.child(label, 1)
}

// removeChild detaches c from n's children. It reports whether c was found.
func (n *Node) removeChild(c *Node) bool {
	for i, ch := range n.Children {
		if ch == c {
			n.Children = append(n.Children[:i], n.Children[i+1:]...)
			c.Parent = nil
			return true
		}
	}
	return false
}

// index returns the 1-based position of n among its siblings sharing its label,
// together with the total count of such siblings. For a node without a parent
// it returns (1, 1).
func (n *Node) index() (pos, count int) {
	if n.Parent == nil {
		return 1, 1
	}
	for _, c := range n.Parent.Children {
		if c.Label == n.Label {
			count++
			if c == n {
				pos = count
			}
		}
	}
	return pos, count
}

// segment returns the path segment for n: its label, plus a [pos] suffix when
// it has same-labelled siblings.
func (n *Node) segment() string {
	pos, count := n.index()
	if count > 1 {
		return n.Label + "[" + strconv.Itoa(pos) + "]"
	}
	return n.Label
}

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package interp

import "strings"

// Tree is a node in the data tree produced by a lens get (or consumed by put).
// A nil Label or Value means "absent", distinct from the empty string, matching
// Augeas semantics.
type Tree struct {
	Label    *string
	Value    *string
	Children []*Tree
}

func strptr(s string) *string { return &s }

// Format renders a forest in the same shape as Augeas tree constants, used for
// diagnostics and test-failure messages.
func Format(forest []*Tree) string {
	var b strings.Builder
	formatForest(&b, forest, 0)
	return b.String()
}

func formatForest(b *strings.Builder, forest []*Tree, indent int) {
	for _, t := range forest {
		for i := 0; i < indent; i++ {
			b.WriteString("  ")
		}
		b.WriteByte('{')
		if t.Label != nil {
			b.WriteString(" \"")
			b.WriteString(*t.Label)
			b.WriteByte('"')
		}
		if t.Value != nil {
			b.WriteString(" = \"")
			b.WriteString(*t.Value)
			b.WriteByte('"')
		}
		if len(t.Children) > 0 {
			b.WriteByte('\n')
			formatForest(b, t.Children, indent+1)
			for i := 0; i < indent; i++ {
				b.WriteString("  ")
			}
		}
		b.WriteString(" }\n")
	}
}

// treesEqual compares two forests for structural equality (label, value,
// children in order).
func treesEqual(a, b []*Tree) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !treeEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func treeEqual(a, b *Tree) bool {
	if !strEqual(a.Label, b.Label) || !strEqual(a.Value, b.Value) {
		return false
	}
	return treesEqual(a.Children, b.Children)
}

func strEqual(a, b *string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	return *a == *b
}

// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"fmt"
	"sort"
)

// Lens converts between the concrete text of a configuration file and a subtree
// in the Augeas tree. Parse turns text into a synthetic parent node whose
// children are the parsed entries; Build performs the reverse. A well-behaved
// lens round-trips: Build(Parse(text)) reproduces canonical text, and
// Parse(Build(node)) reproduces the tree.
type Lens interface {
	Parse(text string) (*Node, error)
	Build(root *Node) (string, error)
}

// registry maps lens names to their implementations.
var registry = map[string]Lens{}

// Register makes lens available under name for LensByName. It panics on a
// duplicate name so wiring mistakes surface at init time.
func Register(name string, lens Lens) {
	if _, ok := registry[name]; ok {
		panic("augeas: lens already registered: " + name)
	}
	registry[name] = lens
}

// LensByName returns the lens registered under name.
func LensByName(name string) (Lens, bool) {
	l, ok := registry[name]
	return l, ok
}

// LensNames returns the sorted names of all registered lenses.
func LensNames() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// TextStore parses text with lens and stores the resulting entries as the
// children of the node at path (creating the path). Any existing children at
// path are replaced.
func (a *Augeas) TextStore(lens Lens, path, text string) error {
	parsed, err := lens.Parse(text)
	if err != nil {
		return a.fail(err)
	}
	dst, err := a.createPath(path)
	if err != nil {
		return a.fail(err)
	}
	dst.Children = nil
	for _, c := range parsed.Children {
		dst.appendChild(c)
	}
	return nil
}

// TextRetrieve serialises a subtree back to text with lens. When node is
// non-nil it is serialised directly; otherwise the single subtree at path is
// used. It mirrors the C aug_text_retrieve signature, in which the caller may
// supply either an explicit node or a tree path.
func (a *Augeas) TextRetrieve(lens Lens, path string, node *Node) (string, error) {
	src := node
	if src == nil {
		nodes, err := a.eval(path)
		if err != nil {
			return "", a.fail(err)
		}
		if len(nodes) != 1 {
			return "", a.fail(fmt.Errorf("text_retrieve path %q matches %d nodes", path, len(nodes)))
		}
		src = nodes[0]
	}
	text, err := lens.Build(src)
	if err != nil {
		return "", a.fail(err)
	}
	return text, nil
}

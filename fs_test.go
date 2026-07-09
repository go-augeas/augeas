// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"io/fs"
	"strings"
	"testing"
)

// fakeFS is an in-memory FileSystem for exercising Load and Save.
type fakeFS struct{ files map[string]string }

func (f fakeFS) ReadFile(n string) ([]byte, error) {
	if s, ok := f.files[n]; ok {
		return []byte(s), nil
	}
	return nil, fs.ErrNotExist
}
func (f fakeFS) WriteFile(n string, d []byte, _ fs.FileMode) error {
	f.files[n] = string(d)
	return nil
}
func (f fakeFS) Glob(p string) ([]string, error) {
	var out []string
	for k := range f.files {
		if strings.HasSuffix(k, strings.TrimPrefix(p, "*")) {
			out = append(out, k)
		}
	}
	return out, nil
}

// lineLens is a trivial Lens used to exercise the generic Load/Save plumbing
// independently of the interpreted corpus.
type lineLens struct{}

func (lineLens) Parse(text string) (*Node, error) {
	root := &Node{Label: "/"}
	for i, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			continue
		}
		n := newNode("line")
		v := line
		n.Value = &v
		_ = i
		root.appendChild(n)
	}
	return root, nil
}
func (lineLens) Build(root *Node) (string, error) {
	var b strings.Builder
	for _, c := range root.Children {
		if c.Value != nil {
			b.WriteString(*c.Value)
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func TestLoadSaveGeneric(t *testing.T) {
	a := New()
	a.SetFileSystem(fakeFS{files: map[string]string{"/etc/demo": "a\nb\n"}})
	if err := a.Load(lineLens{}, "*demo", "/files"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v, ok := a.Get("/files/demo/line[1]"); !ok || v != "a" {
		t.Fatalf("line1 = %q ok=%v", v, ok)
	}
	if err := a.Set("/files/demo/line[2]", "B"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := a.Save(lineLens{}, "/files/demo", "/etc/demo"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := a.fs.(fakeFS).files["/etc/demo"]; got != "a\nB\n" {
		t.Fatalf("saved = %q", got)
	}
}

// nodeVal returns a node's value, or "" if the node or value is absent.
func nodeVal(n *Node) string {
	if n == nil || n.Value == nil {
		return ""
	}
	return *n.Value
}

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

func TestOSFS(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/x.conf"
	var o osFS
	if err := o.WriteFile(f, []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := o.ReadFile(f); err != nil || string(b) != "a\n" {
		t.Fatalf("read %q %v", b, err)
	}
	if m, err := o.Glob(dir + "/*.conf"); err != nil || len(m) != 1 {
		t.Fatalf("glob %v %v", m, err)
	}
}

func TestLoadRecordsError(t *testing.T) {
	a := New()
	a.SetFileSystem(fakeFS{files: map[string]string{"/etc/demo": "x\n"}})
	// a lens whose Parse always errors
	if err := a.Load(errLens{}, "*demo", "/files"); err != nil {
		t.Fatalf("Load returns nil even on per-file error: %v", err)
	}
	if v, _ := a.Get("/augeas/files/demo/error"); v == "" {
		t.Fatal("expected recorded error")
	}
}

type errLens struct{}

func (errLens) Parse(string) (*Node, error) { return nil, errParse }
func (errLens) Build(*Node) (string, error) { return "", errParse }

var errParse = fsErr("boom")

type fsErr string

func (e fsErr) Error() string { return string(e) }

func TestSaveErrors(t *testing.T) {
	a := New()
	a.SetFileSystem(fakeFS{files: map[string]string{}})
	a.Set("/files/d/line[1]", "a")
	// mount matches !=1 -> error (no such mount)
	if err := a.Save(lineLens{}, "/files/nope", "/out"); err == nil {
		t.Fatal("save no-match mount")
	}
	// build error via errLens
	a.Set("/x/y", "z")
	if err := a.Save(errLens{}, "/x/y", "/out"); err == nil {
		t.Fatal("save build error")
	}
	// successful save
	if err := a.Save(lineLens{}, "/files/d", "/out"); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestLoadGlobError(t *testing.T) {
	a := New()
	a.SetFileSystem(errGlobFS{})
	if err := a.Load(lineLens{}, "*", "/files"); err == nil {
		t.Fatal("expected glob error")
	}
}

type errGlobFS struct{}

func (errGlobFS) ReadFile(string) ([]byte, error)             { return nil, fsErr("x") }
func (errGlobFS) WriteFile(string, []byte, fs.FileMode) error { return nil }
func (errGlobFS) Glob(string) ([]string, error)               { return nil, fsErr("glob boom") }

func TestLoadReadError(t *testing.T) {
	a := New()
	a.SetFileSystem(readErrFS{})
	// glob returns a file, read fails -> recorded error, Load returns nil
	if err := a.Load(lineLens{}, "*x", "/files"); err != nil {
		t.Fatalf("load: %v", err)
	}
	if v, _ := a.Get("/augeas/files/x/error"); v == "" {
		t.Fatal("expected recorded read error")
	}
}

type readErrFS struct{}

func (readErrFS) ReadFile(string) ([]byte, error)             { return nil, fsErr("read boom") }
func (readErrFS) WriteFile(string, []byte, fs.FileMode) error { return nil }
func (readErrFS) Glob(string) ([]string, error)               { return []string{"/etc/x"}, nil }

func TestTrimLeadingSlash(t *testing.T) {
	if trimLeadingSlash("/a") != "a" || trimLeadingSlash("a") != "a" {
		t.Fatal("trimLeadingSlash")
	}
}

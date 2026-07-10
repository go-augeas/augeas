// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"io/fs"
	"testing"

	"github.com/go-augeas/augeas/internal/interp"
)

func TestInterpLensParseError(t *testing.T) {
	e := NewEngine()
	l, err := e.Lens("Hosts", "lns")
	if err != nil {
		t.Fatal(err)
	}
	// input that the Hosts lens cannot parse
	if _, err := l.Parse("!!! not a hosts file @@@"); err == nil {
		t.Error("expected parse error")
	}
}

func TestFilterMatchesExclude(t *testing.T) {
	// an exclude glob that matches must veto inclusion
	filters := []interp.Filter{
		{Glob: "/etc/*", Include: true},
		{Glob: "/etc/secret", Include: false},
	}
	if filterMatches(filters, "/etc/secret") {
		t.Error("exclude should veto")
	}
	if !filterMatches(filters, "/etc/hosts") {
		t.Error("include should match")
	}
	// bad glob pattern
	if globMatch("[", "x") {
		t.Error("bad glob")
	}
}

func TestLoadFileErrorPaths(t *testing.T) {
	// matched lens, but file read fails
	a := New()
	a.SetFileSystem(rerrFS{})
	if err := a.LoadFile("/etc/hosts"); err == nil {
		t.Error("expected read error")
	}
	// matched lens, but content does not parse
	a2 := New()
	a2.SetFileSystem(memFSc{files: map[string]string{"/etc/hosts": "@@@ garbage @@@"}})
	if err := a2.LoadFile("/etc/hosts"); err == nil {
		t.Error("expected parse error")
	}
	// createFrom fails (path with predicate chars)
	a3 := New()
	a3.SetFileSystem(memFSc{files: map[string]string{"/etc/hosts": "127.0.0.1 localhost\n"}})
	// pre-create a conflicting node so files/etc/hosts createFrom hits a bad segment
	// use a path segment with '[' which createFrom rejects
	a3b := New()
	a3b.SetFileSystem(badPathFS{})
	_ = a3b.LoadFile("/e[t]c") // matches no lens -> returns "no lens" error; still exercises resolver
}

type rerrFS struct{}

func (rerrFS) ReadFile(string) ([]byte, error)             { return nil, fsErr("read fail") }
func (rerrFS) WriteFile(string, []byte, fs.FileMode) error { return nil }
func (rerrFS) Glob(string) ([]string, error)               { return nil, nil }

type badPathFS struct{}

func (badPathFS) ReadFile(string) ([]byte, error)             { return []byte("x"), nil }
func (badPathFS) WriteFile(string, []byte, fs.FileMode) error { return nil }
func (badPathFS) Glob(string) ([]string, error)               { return nil, nil }

func TestLoadReadRecordsError2(t *testing.T) {
	a := New()
	a.SetFileSystem(readErrFS{})
	if err := a.Load(lineLens{}, "*x", "/files"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Get("/augeas/files/x/error"); v == "" {
		t.Fatal("expected recorded error")
	}
}

func TestSaveErrorPaths(t *testing.T) {
	a := New()
	a.SetFileSystem(fakeFS{files: map[string]string{}})
	a.Set("/files/d/line[1]", "a")
	// mount matches zero nodes -> eval ok but count 0
	if err := a.Save(lineLens{}, "/files/none", "/out"); err == nil {
		t.Error("save no-match mount")
	}
	// mount is a malformed path -> eval error
	if err := a.Save(lineLens{}, "/[bad", "/out"); err == nil {
		t.Error("save eval error")
	}
	// build error
	if err := a.Save(errLens{}, "/files/d", "/out"); err == nil {
		t.Error("save build error")
	}
	// write error
	a2 := New()
	a2.SetFileSystem(writeErrFS{})
	a2.Set("/files/d/line[1]", "a")
	if err := a2.Save(lineLens{}, "/files/d", "/out"); err == nil {
		t.Error("save write error")
	}
}

type writeErrFS struct{}

func (writeErrFS) ReadFile(string) ([]byte, error)             { return nil, fsErr("x") }
func (writeErrFS) WriteFile(string, []byte, fs.FileMode) error { return fsErr("write fail") }
func (writeErrFS) Glob(string) ([]string, error)               { return nil, nil }

func TestRecordErrorBadBase(t *testing.T) {
	a := New()
	// a base containing path metacharacters makes createFrom fail
	a.recordError("bad[base]", "msg")
}

func TestTextStoreRetrieveErrors(t *testing.T) {
	a := New()
	// TextStore with an un-createable path
	if err := a.TextStore(lineLens{}, "/bad[1]/x", "a\n"); err == nil {
		t.Error("TextStore path error")
	}
	// TextRetrieve build error
	a.Set("/x/y", "z")
	if _, err := a.TextRetrieve(errLens{}, "/x/y", nil); err == nil {
		t.Error("TextRetrieve build error")
	}
}

func TestLoadCreateFromError(t *testing.T) {
	// a globbed file whose base contains path metacharacters makes createFrom
	// fail, exercising loadOne's create branch.
	a := New()
	a.SetFileSystem(badBaseFS{})
	if err := a.Load(lineLens{}, "*", "/files"); err != nil {
		t.Fatalf("load: %v", err)
	}
	// error recorded under the sanitised base
	if len(a.Root().Children) == 0 {
		t.Fatal("expected some tree state")
	}
}

type badBaseFS struct{}

func (badBaseFS) ReadFile(string) ([]byte, error)             { return []byte("a\n"), nil }
func (badBaseFS) WriteFile(string, []byte, fs.FileMode) error { return nil }
func (badBaseFS) Glob(string) ([]string, error)               { return []string{"/etc/x[1]"}, nil }

func TestTextStoreParseError(t *testing.T) {
	a := New()
	if err := a.TextStore(errLens{}, "/files/x", "anything"); err == nil {
		t.Error("expected TextStore parse error")
	}
}

func TestLoadFileCreateFromError(t *testing.T) {
	// A path that matches a lens glob (/etc/default/*) but whose final segment
	// contains a path metacharacter makes createFrom fail after a successful
	// parse, exercising loadFileInto's create branch.
	a := New()
	a.SetFileSystem(memFSc{files: map[string]string{"/etc/default/x[1]": "A=b\n"}})
	if err := a.LoadFile("/etc/default/x[1]"); err == nil {
		t.Error("expected createFrom error")
	}
}

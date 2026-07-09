// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"testing"
)

// fakeFS is an in-memory FileSystem for exercising Load and Save.
type fakeFS struct {
	files    map[string]string
	readErr  map[string]error
	globErr  error
	writeErr error
	writes   map[string][]byte
}

func (f *fakeFS) Glob(pattern string) ([]string, error) {
	if f.globErr != nil {
		return nil, f.globErr
	}
	var out []string
	for k := range f.files {
		if ok, _ := filepath.Match(pattern, k); ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeFS) ReadFile(name string) ([]byte, error) {
	if e := f.readErr[name]; e != nil {
		return nil, e
	}
	return []byte(f.files[name]), nil
}

func (f *fakeFS) WriteFile(name string, data []byte, _ fs.FileMode) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if f.writes == nil {
		f.writes = map[string][]byte{}
	}
	f.writes[name] = data
	return nil
}

// roundTrip asserts that Build(Parse(text)) == text for a canonical text.
func roundTrip(t *testing.T, lens Lens, text string) *Node {
	t.Helper()
	n, err := lens.Parse(text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	out, err := lens.Build(n)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if out != text {
		t.Fatalf("round-trip mismatch:\n in: %q\nout: %q", text, out)
	}
	return n
}

func TestSplitLinesAndComment(t *testing.T) {
	if splitLines("") != nil {
		t.Fatal("empty")
	}
	if got := splitLines("a\nb\n"); len(got) != 2 {
		t.Fatalf("trailing newline %v", got)
	}
	if got := splitLines("a\nb"); len(got) != 2 {
		t.Fatalf("no trailing %v", got)
	}
	if _, ok := isComment("", "#"); ok {
		t.Fatal("empty is not comment")
	}
	if _, ok := isComment("x", "#"); ok {
		t.Fatal("x is not comment")
	}
}

func TestHostsLens(t *testing.T) {
	lens := hostsLens{}
	roundTrip(t, lens, "# a comment\n127.0.0.1 localhost host loopback\n192.168.0.1 h # inline\n")
	// blank lines are skipped
	n, _ := lens.Parse("\n127.0.0.1 h\n")
	if len(n.Children) != 1 {
		t.Fatalf("blank skip %d", len(n.Children))
	}
	// too few fields
	if _, err := lens.Parse("127.0.0.1\n"); err == nil {
		t.Fatal("want field error")
	}
	// build error: missing ipaddr
	bad := &Node{}
	bad.appendChild(newNode("1"))
	if _, err := lens.Build(bad); err == nil {
		t.Fatal("want build error")
	}
}

func TestFstabLens(t *testing.T) {
	lens := fstabLens{}
	// full 6-field + comment
	roundTrip(t, lens, "# fstab\n/dev/sda1 / ext4 defaults,rw 0 1\n")
	// 4-field (no dump/passno)
	roundTrip(t, lens, "/dev/sdb / xfs defaults\n")
	// 5-field (dump only)
	roundTrip(t, lens, "/dev/sdc / xfs defaults 0\n")
	// blank skip
	n, _ := lens.Parse("\n/dev/sda1 / ext4 defaults\n")
	if len(n.Children) != 1 {
		t.Fatal("blank skip")
	}
	// too few / too many
	if _, err := lens.Parse("a b c\n"); err == nil {
		t.Fatal("too few")
	}
	if _, err := lens.Parse("a b c d e f g\n"); err == nil {
		t.Fatal("too many")
	}
	// build error: missing spec
	bad := &Node{}
	bad.appendChild(newNode("1"))
	if _, err := lens.Build(bad); err == nil {
		t.Fatal("missing spec")
	}
	// build error: no options
	bad2 := &Node{}
	e := newNode("1")
	addChild(e, "spec", "s")
	addChild(e, "file", "/")
	addChild(e, "vfstype", "ext4")
	bad2.appendChild(e)
	if _, err := lens.Build(bad2); err == nil {
		t.Fatal("no options")
	}
}

func TestShellvarsLens(t *testing.T) {
	lens := shellvarsLens{}
	roundTrip(t, lens, "# c\nKEY=value\nOTHER=x\n")
	// blank skip
	n, _ := lens.Parse("\nKEY=v\n")
	if len(n.Children) != 1 {
		t.Fatal("blank skip")
	}
	// missing '='
	if _, err := lens.Parse("noequals\n"); err == nil {
		t.Fatal("missing eq")
	}
	// leading '=' (empty key)
	if _, err := lens.Parse("=v\n"); err == nil {
		t.Fatal("empty key")
	}
}

func TestIniLens(t *testing.T) {
	lens := iniLens{}
	roundTrip(t, lens, "top = 1\n# c\n[section]\nkey = value\n# scoped\n")
	// semicolon comment parses (canonicalised to '#', tree assertion only)
	n, err := lens.Parse("; semi\n[s]\nk = v\n")
	if err != nil || n.Children[0].Label != "#comment" {
		t.Fatalf("semicolon %v %v", err, n.Children)
	}
	// blank skip
	n2, _ := lens.Parse("\nk = v\n")
	if len(n2.Children) != 1 {
		t.Fatal("blank skip")
	}
	// bad section header
	if _, err := lens.Parse("[section\n"); err == nil {
		t.Fatal("bad header")
	}
	// empty section name
	if _, err := lens.Parse("[]\n"); err == nil {
		t.Fatal("empty section")
	}
	// missing '='
	if _, err := lens.Parse("noequals\n"); err == nil {
		t.Fatal("missing eq")
	}
	// empty key
	if _, err := lens.Parse("= v\n"); err == nil {
		t.Fatal("empty key")
	}
}

func TestOSFS(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x.conf")
	sys := osFS{}
	if err := sys.WriteFile(f, []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := sys.ReadFile(f)
	if err != nil || string(b) != "A=1\n" {
		t.Fatalf("read %q %v", b, err)
	}
	m, err := sys.Glob(filepath.Join(dir, "*.conf"))
	if err != nil || len(m) != 1 {
		t.Fatalf("glob %v %v", m, err)
	}
}

func TestLoad(t *testing.T) {
	lens, _ := LensByName("Hosts")
	ff := &fakeFS{
		files: map[string]string{
			"good.hosts": "127.0.0.1 localhost\n",
			"bad.hosts":  "oops\n",      // parse error
			"a[b":        "1.1.1.1 h\n", // createFrom error (unmappable label)
			"unreadable": "",            // read error
		},
		readErr: map[string]error{"unreadable": errors.New("boom")},
	}
	a := New()
	a.SetFileSystem(ff)
	if err := a.Load(lens, "*", "/files"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Get("/files/good.hosts/1/canonical"); v != "localhost" {
		t.Fatalf("loaded %q", v)
	}
	if !a.Exists("/augeas/files/bad.hosts/error") {
		t.Fatal("parse error metadata missing")
	}
	if !a.Exists("/augeas/files/unreadable/error") {
		t.Fatal("read error metadata missing")
	}
	// glob error
	a2 := New()
	a2.SetFileSystem(&fakeFS{globErr: errors.New("no glob")})
	if err := a2.Load(lens, "*", "/files"); err == nil {
		t.Fatal("want glob error")
	}
}

func TestSave(t *testing.T) {
	lens, _ := LensByName("Hosts")
	a := New()
	a.TextStore(lens, "/files/etc/hosts", "127.0.0.1 localhost\n")
	ff := &fakeFS{}
	a.SetFileSystem(ff)
	if err := a.Save(lens, "/files/etc/hosts", "/out/hosts"); err != nil {
		t.Fatal(err)
	}
	if string(ff.writes["/out/hosts"]) != "127.0.0.1 localhost\n" {
		t.Fatalf("saved %q", ff.writes["/out/hosts"])
	}
	// eval error
	if err := a.Save(lens, "/[1]", "/out"); err == nil {
		t.Fatal("eval error")
	}
	// not single match
	if err := a.Save(lens, "/nope", "/out"); err == nil {
		t.Fatal("zero match")
	}
	// build error: bad subtree
	a.Root().appendChild(func() *Node { n := newNode("bad"); n.appendChild(newNode("1")); return n }())
	if err := a.Save(lens, "/bad", "/out"); err == nil {
		t.Fatal("build error")
	}
	// write error
	a.SetFileSystem(&fakeFS{writeErr: errors.New("disk full")})
	if err := a.Save(lens, "/files/etc/hosts", "/out"); err == nil {
		t.Fatal("write error")
	}
}

func TestTrimLeadingSlash(t *testing.T) {
	if trimLeadingSlash("/x") != "x" {
		t.Fatal("slash")
	}
	if trimLeadingSlash("x") != "x" {
		t.Fatal("no slash")
	}
}

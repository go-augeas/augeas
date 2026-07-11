// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"io/fs"
	"testing"

	"github.com/go-augeas/augeas/internal/interp"
)

func TestEngineLens(t *testing.T) {
	e := NewEngine()
	l, err := e.Lens("Hosts", "lns")
	if err != nil {
		t.Fatalf("lens: %v", err)
	}
	if _, err := l.Parse("127.0.0.1 localhost\n"); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := l.Build(&Node{}); err == nil {
		t.Fatal("Build must be unsupported")
	}
	if _, err := e.Lens("Nope", "lns"); err == nil {
		t.Fatal("missing module")
	}
	if _, err := e.Lens("Hosts", "nope"); err == nil {
		t.Fatal("missing binding")
	}
}

func TestFromInterpTree(t *testing.T) {
	tr := &interp.Tree{Children: []*interp.Tree{{Label: nil, Value: nil}}}
	n := fromInterpTree(tr)
	if n.Label != "" || n.Value != nil || len(n.Children) != 1 {
		t.Fatalf("node: %+v", n)
	}
}

func TestFilterAndGlob(t *testing.T) {
	filters := []interp.Filter{
		{Glob: "/etc/hosts", Include: true},
		{Glob: "/etc/hosts.bak", Include: false},
	}
	if !filterMatches(filters, "/etc/hosts") {
		t.Fatal("should include")
	}
	if filterMatches(filters, "/etc/hosts.bak") {
		t.Fatal("should exclude")
	}
	if filterMatches(filters, "/other") {
		t.Fatal("not included")
	}
	if !globMatch("/a/*.conf", "/a/x.conf") {
		t.Fatal("glob match")
	}
	if globMatch("[", "x") {
		t.Fatal("bad glob")
	}
}

func TestEnsureSlash(t *testing.T) {
	if ensureSlash("a") != "/a" || ensureSlash("/a") != "/a" {
		t.Fatal("ensureSlash")
	}
}

func TestLoadFileErrors(t *testing.T) {
	// no lens for path
	a := New()
	a.SetFileSystem(memFSc{files: map[string]string{}})
	if err := a.LoadFile("/no/such/lens/target"); err == nil {
		t.Fatal("no lens")
	}
	// read error (file matches a lens glob but is absent)
	a2 := New()
	a2.SetFileSystem(memFSc{files: map[string]string{}})
	if err := a2.LoadFile("/etc/hosts"); err == nil {
		t.Fatal("read error")
	}
	// successful load
	a3 := New()
	a3.SetFileSystem(memFSc{files: map[string]string{"/etc/hosts": "127.0.0.1 localhost\n"}})
	if err := a3.LoadFile("/etc/hosts"); err != nil {
		t.Fatalf("load: %v", err)
	}
	if v, _ := a3.Get("/files/etc/hosts/1/ipaddr"); v != "127.0.0.1" {
		t.Fatalf("ipaddr %q", v)
	}
}

// memFSc is a minimal in-memory filesystem for the corpus tests.
type memFSc struct{ files map[string]string }

func (m memFSc) ReadFile(n string) ([]byte, error) {
	if s, ok := m.files[n]; ok {
		return []byte(s), nil
	}
	return nil, fs.ErrNotExist
}
func (m memFSc) WriteFile(n string, d []byte, _ fs.FileMode) error {
	m.files[n] = string(d)
	return nil
}
func (m memFSc) Glob(string) ([]string, error) { return nil, nil }

// TestCaddyfileHeredocGuard verifies the API-boundary rejection of Caddyfile
// heredocs, which Caddyfile.lns cannot model and would otherwise silently
// misparse. Both entry points (direct Engine.Lens Parse and the LoadFile
// autoload path) must refuse a heredoc file and accept a normal one.
func TestCaddyfileHeredocGuard(t *testing.T) {
	const heredoc = "example.com {\n\trespond <<HTML\n\t<h1>hi</h1>\n\tHTML 200\n}\n"
	const normal = "example.com {\n\treverse_proxy localhost:8080\n}\n"

	// Direct-binding path: lens name == "Caddyfile.lns".
	l, err := NewEngine().Lens("Caddyfile", "lns")
	if err != nil {
		t.Fatalf("lens: %v", err)
	}
	if _, err := l.Parse(heredoc); err == nil {
		t.Fatal("heredoc input must be rejected by Parse")
	}
	if _, err := l.Parse(normal); err != nil {
		t.Fatalf("normal input must parse: %v", err)
	}

	// Autoload / LoadFile path: lens name == "Caddyfile".
	a := New()
	a.SetFileSystem(memFSc{files: map[string]string{"/etc/caddy/Caddyfile": heredoc}})
	if err := a.LoadFile("/etc/caddy/Caddyfile"); err == nil {
		t.Fatal("LoadFile must reject a heredoc Caddyfile")
	}
	a2 := New()
	a2.SetFileSystem(memFSc{files: map[string]string{"/etc/caddy/Caddyfile": normal}})
	if err := a2.LoadFile("/etc/caddy/Caddyfile"); err != nil {
		t.Fatalf("LoadFile must parse a normal Caddyfile: %v", err)
	}
	if v, _ := a2.Get("/files/etc/caddy/Caddyfile/example.com/reverse_proxy/arg"); v != "localhost:8080" {
		t.Fatalf("parsed arg = %q, want localhost:8080", v)
	}

	// The guard is scoped to the Caddyfile lens: a `<<` opener in another
	// lens' input is not spuriously rejected.
	if err := checkCaddyfileHeredoc("plain config, no heredoc\n"); err != nil {
		t.Fatalf("non-heredoc input flagged: %v", err)
	}
}

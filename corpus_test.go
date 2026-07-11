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

// TestNftablesLineWrapGuard verifies the API-boundary rejection of nftables
// rulesets whose brace groups wrap across physical lines (which Nftables.lns
// would silently misparse into bogus sibling rule nodes), and that every
// legitimate line shape is accepted.
func TestNftablesLineWrapGuard(t *testing.T) {
	const normal = "table inet filter {\n\tchain input {\n\t\ttype filter hook input priority 0; policy drop;\n\t\ttcp dport { 22, 80 } accept\n\t}\n}\n"
	// nft-wrapped element list: the "{" on the elements line is not closed on
	// the same line.
	const wrapOpen = "table ip filter {\n\tset big {\n\t\telements = { 10.0.0.1,\n\t\t\t     10.0.0.2 }\n\t}\n}\n"

	l, err := NewEngine().Lens("Nftables", "lns")
	if err != nil {
		t.Fatalf("lens: %v", err)
	}
	if _, err := l.Parse(wrapOpen); err == nil {
		t.Fatal("wrapped element-list opener must be rejected by Parse")
	}
	if _, err := l.Parse(normal); err != nil {
		t.Fatalf("normal ruleset must parse: %v", err)
	}

	// Autoload / LoadFile path: lens name == "Nftables".
	a := New()
	a.SetFileSystem(memFSc{files: map[string]string{"/etc/nftables.conf": wrapOpen}})
	if err := a.LoadFile("/etc/nftables.conf"); err == nil {
		t.Fatal("LoadFile must reject a wrapped nftables ruleset")
	}
	a2 := New()
	a2.SetFileSystem(memFSc{files: map[string]string{"/etc/nftables.conf": normal}})
	if err := a2.LoadFile("/etc/nftables.conf"); err != nil {
		t.Fatalf("LoadFile must parse a normal ruleset: %v", err)
	}
	if v, _ := a2.Get("/files/etc/nftables.conf/table/name"); v != "filter" {
		t.Fatalf("parsed table name = %q, want filter", v)
	}

	// Direct guard checks of the accept branches: blank lines, a comment-only
	// line (dropped at "#"), a lone closer "}", a balanced inline set, and a
	// block opener ending in "{" must all pass.
	if err := checkNftablesLineWrap("\n# a comment with { and } inside\ntable inet f {\n\ttcp dport { 22, 80 } accept\n}\n"); err != nil {
		t.Fatalf("legitimate lines flagged: %v", err)
	}
	// More than one unmatched "{" on a line is also a wrap (depth > 1).
	if err := checkNftablesLineWrap("a { b {\n"); err == nil {
		t.Fatal("a line with two unmatched { must be rejected")
	}
	// A stray "}" continuing a previous line (depth goes negative) is rejected.
	if err := checkNftablesLineWrap("10.0.0.2 } accept\n"); err == nil {
		t.Fatal("a line continuing a brace group with a stray } must be rejected")
	}
}

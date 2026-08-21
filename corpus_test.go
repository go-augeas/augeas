// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/go-augeas/augeas/internal/interp"
)

func TestEngineLens(t *testing.T) {
	e := NewEngine()
	l, err := e.Lens("Hosts", "lns")
	if err != nil {
		t.Fatalf("lens: %v", err)
	}
	const src = "127.0.0.1 localhost\n"
	root, err := l.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Build used to be unsupported for interpreted lenses. It now round-trips
	// through the skeleton Parse recorded, so the original spacing survives.
	out, err := l.Build(root)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if out != src {
		t.Fatalf("round trip = %q, want %q", out, src)
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

// TestBuildSurface covers what the round trip in TestEngineLens does not: a
// tree the lens cannot serialise, a nil child, and a nil root.
func TestBuildSurface(t *testing.T) {
	e := NewEngine()
	l, err := e.Lens("Hosts", "lns")
	if err != nil {
		t.Fatalf("lens: %v", err)
	}

	// A record with no children does not match the lens schema. put must fail,
	// and the error must name the lens rather than surfacing a bare
	// interpreter message.
	bad := &Node{Label: "/"}
	bad.appendChild(&Node{Label: "1"})
	_, err = l.Build(bad)
	if err == nil {
		t.Fatal("Build of a tree that does not match the schema must fail")
	}
	if !strings.Contains(err.Error(), `Hosts.lns`) {
		t.Errorf("error should name the lens, got %v", err)
	}

	// A nil child is skipped rather than forwarded: a nil *interp.Tree in the
	// forest dereferences inside the interpreter.
	if _, err := l.Build(&Node{Label: "/", Children: []*Node{nil}}); err != nil {
		t.Errorf("nil child: %v", err)
	}

	// A nil root puts an empty forest.
	if _, err := l.Build(nil); err != nil {
		t.Errorf("nil root: %v", err)
	}

	// A child carrying a value exercises the value branch of toInterpTree.
	withVal := &Node{Label: "/"}
	rec := &Node{Label: "1"}
	rec.appendChild(&Node{Label: "ipaddr", Value: strPtr("127.0.0.1")})
	rec.appendChild(&Node{Label: "canonical", Value: strPtr("localhost")})
	withVal.appendChild(rec)
	if out, err := l.Build(withVal); err != nil {
		t.Errorf("build a well-formed record: %v", err)
	} else if !strings.Contains(out, "127.0.0.1") {
		t.Errorf("output lost the address: %q", out)
	}
}

func strPtr(s string) *string { return &s }

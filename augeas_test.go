// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"reflect"
	"testing"
)

// build sets up a small tree used across tests.
func build(t *testing.T) *Augeas {
	t.Helper()
	a := New()
	if err := a.Set("/files/etc/hosts/1/ipaddr", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := a.Set("/files/etc/hosts/1/canonical", "localhost"); err != nil {
		t.Fatal(err)
	}
	if err := a.Set("/files/etc/hosts/1/alias[1]", "loopback"); err != nil {
		// alias[1] with a positional index on a fresh path is not creatable;
		// use plain labels instead below.
		_ = err
	}
	if err := a.Set("/files/etc/hosts/2/ipaddr", "192.168.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := a.Set("/files/etc/hosts/2/canonical", "host.example.com"); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestNodeBasics(t *testing.T) {
	n := newNode("x")
	if n.Value != nil {
		t.Fatal("new node should be valueless")
	}
	n.SetValue("v")
	if nodeVal(n) != "v" {
		t.Fatalf("got %q", nodeVal(n))
	}
	n.ClearValue()
	if n.Value != nil {
		t.Fatal("ClearValue failed")
	}
	// child indexing
	p := newNode("p")
	for range 3 {
		p.appendChild(newNode("a"))
	}
	if p.child("a", 2) == nil {
		t.Fatal("want 2nd a")
	}
	if p.child("a", 5) != nil {
		t.Fatal("index 5 should miss")
	}
	if p.firstChild("a") != p.Children[0] {
		t.Fatal("firstChild")
	}
	// removeChild not-found
	if p.removeChild(newNode("stranger")) {
		t.Fatal("removing a stranger must fail")
	}
	// detached node index/segment
	d := newNode("d")
	if pos, count := d.index(); pos != 1 || count != 1 {
		t.Fatalf("detached index %d/%d", pos, count)
	}
	if d.segment() != "d" {
		t.Fatal("detached segment")
	}
	// nodeVal nil node
	if nodeVal(nil) != "" {
		t.Fatal("nil nodeVal")
	}
}

func TestGetSetExists(t *testing.T) {
	a := build(t)
	if v, ok := a.Get("/files/etc/hosts/1/ipaddr"); !ok || v != "127.0.0.1" {
		t.Fatalf("get %q %v", v, ok)
	}
	if _, ok := a.Get("/files/etc/hosts/9/ipaddr"); ok {
		t.Fatal("missing get should be false")
	}
	// multiple match
	if _, ok := a.Get("/files/etc/hosts/*/ipaddr"); ok {
		t.Fatal("multi get should be false")
	}
	if a.Error() == nil {
		t.Fatal("multi get should set error")
	}
	// malformed path
	if _, ok := a.Get("/[1]"); ok {
		t.Fatal("bad path get")
	}
	// valueless node via Insert
	if err := a.Insert("/files/etc/hosts/1/ipaddr", "before", true); err != nil {
		t.Fatal(err)
	}
	if v, ok := a.Get("/files/etc/hosts/1/before"); !ok || v != "" {
		t.Fatalf("valueless get %q %v", v, ok)
	}
	if !a.Exists("/files/etc/hosts/1") {
		t.Fatal("exists")
	}
	if a.Exists("/nope") {
		t.Fatal("not exists")
	}
	if a.Exists("/[1]") {
		t.Fatal("bad path exists")
	}
}

func TestSetPaths(t *testing.T) {
	a := New()
	// create
	if err := a.Set("/a/b/c", "1"); err != nil {
		t.Fatal(err)
	}
	// overwrite (single match)
	if err := a.Set("/a/b/c", "2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Get("/a/b/c"); v != "2" {
		t.Fatalf("got %q", v)
	}
	// bad path
	if err := a.Set("/[1]", "x"); err == nil {
		t.Fatal("want error")
	}
	// create with uncreatable segment
	if err := a.Set("/x[1]/y", "z"); err == nil {
		t.Fatal("want create error")
	}
	// create with a relative path (not creatable)
	if err := a.Set("relative/path", "z"); err == nil {
		t.Fatal("want relative create error")
	}
	// multiple match
	a.Set("/m/n", "1")
	a.Set("/m/n2", "1")
	if err := a.Set("/m/*", "z"); err == nil {
		t.Fatal("want multi error")
	}
}

func TestSetMultiple(t *testing.T) {
	a := New()
	a.Set("/svc/1/state", "on")
	a.Set("/svc/2/state", "on")
	a.Set("/svc/3/other", "x")
	n, err := a.SetMultiple("/svc/*", "state", "off")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 { // 1,2 existing + created under 3
		t.Fatalf("count %d", n)
	}
	if v, _ := a.Get("/svc/3/state"); v != "off" {
		t.Fatalf("created %q", v)
	}
	// base error
	if _, err := a.SetMultiple("/[1]", "x", "y"); err == nil {
		t.Fatal("want base error")
	}
	// sub parse error
	if _, err := a.SetMultiple("/svc/*", "[1]", "y"); err == nil {
		t.Fatal("want sub parse error")
	}
	// sub create error
	if _, err := a.SetMultiple("/svc/*", "a[1]/b", "y"); err == nil {
		t.Fatal("want sub create error")
	}
}

func TestInsert(t *testing.T) {
	a := New()
	a.Set("/p/a", "1")
	a.Set("/p/b", "2")
	// after a non-first node, before=false
	if err := a.Insert("/p/b", "c", false); err != nil {
		t.Fatal(err)
	}
	// before first
	if err := a.Insert("/p/a", "z", true); err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	for _, ch := range a.Root().firstChild("p").Children {
		labels = append(labels, ch.Label)
	}
	if !reflect.DeepEqual(labels, []string{"z", "a", "b", "c"}) {
		t.Fatalf("order %v", labels)
	}
	// invalid label
	if err := a.Insert("/p/a", "bad/label", false); err == nil {
		t.Fatal("want label error")
	}
	// bad path
	if err := a.Insert("/[1]", "x", false); err == nil {
		t.Fatal("want path error")
	}
	// zero match
	if err := a.Insert("/nope", "x", false); err == nil {
		t.Fatal("want zero-match error")
	}
	// next to root
	if err := a.Insert("/", "x", false); err == nil {
		t.Fatal("want root error")
	}
}

func TestRemove(t *testing.T) {
	a := build(t)
	if n := a.Remove("/files/etc/hosts/*"); n != 2 {
		t.Fatalf("removed %d", n)
	}
	if a.Exists("/files/etc/hosts/1") {
		t.Fatal("should be gone")
	}
	// bad path
	if n := a.Remove("/[1]"); n != 0 {
		t.Fatal("bad path remove")
	}
	// root
	if n := a.Remove("/"); n != 0 {
		t.Fatal("root remove")
	}
}

func TestMove(t *testing.T) {
	a := New()
	a.Set("/src/child", "v")
	a.Set("/dst", "old")
	// overwrite existing dst
	if err := a.Move("/src", "/dst"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Get("/dst/child"); v != "v" {
		t.Fatalf("moved child %q", v)
	}
	if a.Exists("/src") {
		t.Fatal("src should be gone")
	}
	// create dst
	a.Set("/s2/c", "1")
	if err := a.Move("/s2", "/new/place"); err != nil {
		t.Fatal(err)
	}
	if !a.Exists("/new/place/c") {
		t.Fatal("moved to created dst")
	}
	// src bad path
	if err := a.Move("/[1]", "/x"); err == nil {
		t.Fatal("src bad path")
	}
	// src not single
	a.Set("/multi/a", "1")
	a.Set("/multi/b", "1")
	if err := a.Move("/multi/*", "/x"); err == nil {
		t.Fatal("src multi")
	}
	// src root
	if err := a.Move("/", "/x"); err == nil {
		t.Fatal("src root")
	}
	// dst bad path
	if err := a.Move("/dst", "/[1]"); err == nil {
		t.Fatal("dst bad path")
	}
	// dst multi
	if err := a.Move("/dst", "/multi/*"); err == nil {
		t.Fatal("dst multi")
	}
	// dst == src
	if err := a.Move("/dst", "/dst"); err == nil {
		t.Fatal("dst equals src")
	}
	// dst inside src
	a.Set("/tree/leaf", "1")
	if err := a.Move("/tree", "/tree/leaf"); err == nil {
		t.Fatal("dst descendant")
	}
	// dst create fail
	if err := a.Move("/dst", "/z[1]/y"); err == nil {
		t.Fatal("dst create fail")
	}
}

func TestMatchLabelVars(t *testing.T) {
	a := build(t)
	// add aliases so segment index shows up
	a.Set("/files/etc/hosts/1/alias", "a1")
	a.Insert("/files/etc/hosts/1/alias", "alias", false)
	m := a.Match("/files/etc/hosts/1/alias")
	if len(m) != 2 || m[0] != "/files/etc/hosts/1/alias[1]" {
		t.Fatalf("match %v", m)
	}
	// relative path
	if r := a.Match("files"); len(r) != 1 || r[0] != "/files" {
		t.Fatalf("relative %v", r)
	}
	// descendant + union + dedup
	u := a.Match("/files/etc/hosts/1/ipaddr | /files/etc/hosts/1/ipaddr")
	if len(u) != 1 {
		t.Fatalf("union dedup %v", u)
	}
	if d := a.Match("/files//canonical"); len(d) != 2 {
		t.Fatalf("descendant %v", d)
	}
	// bad path
	if r := a.Match("/[1]"); r != nil {
		t.Fatal("bad match")
	}
	// Label
	if l, ok := a.Label("/files/etc/hosts/1/ipaddr"); !ok || l != "ipaddr" {
		t.Fatalf("label %q %v", l, ok)
	}
	if _, ok := a.Label("/[1]"); ok {
		t.Fatal("bad label path")
	}
	if _, ok := a.Label("/files/etc/hosts/*/ipaddr"); ok {
		t.Fatal("multi label")
	}
}

func TestVariables(t *testing.T) {
	a := build(t)
	n, err := a.DefineVariable("hosts", "/files/etc/hosts/*")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("defvar %d", n)
	}
	// use var with trailing path
	if m := a.Match("$hosts/ipaddr"); len(m) != 2 {
		t.Fatalf("var path %v", m)
	}
	// use var alone
	if m := a.Match("$hosts"); len(m) != 2 {
		t.Fatalf("var alone %v", m)
	}
	// undefined var
	if m := a.Match("$nope"); m != nil {
		t.Fatal("undefined var")
	}
	// defvar error
	if _, err := a.DefineVariable("x", "/[1]"); err == nil {
		t.Fatal("defvar bad expr")
	}
}

func TestDefineNode(t *testing.T) {
	a := New()
	a.Set("/existing", "v")
	// existing
	p, created := a.DefineNode("v", "/existing", "def")
	if created || p != "/existing" {
		t.Fatalf("existing defnode %q %v", p, created)
	}
	// create
	p, created = a.DefineNode("v2", "/made/here", "def")
	if !created || p != "/made/here" {
		t.Fatalf("created defnode %q %v", p, created)
	}
	if v, _ := a.Get("/made/here"); v != "def" {
		t.Fatalf("defnode value %q", v)
	}
	// eval error
	if _, ok := a.DefineNode("v3", "/[1]", "x"); ok {
		t.Fatal("defnode eval error")
	}
	// create error
	if p, ok := a.DefineNode("v4", "/z[1]/y", "x"); ok || p != "" {
		t.Fatal("defnode create error")
	}
}

func TestSpanAndFS(t *testing.T) {
	a := New()
	if _, err := a.Span("/x"); err != ErrSpanUnsupported {
		t.Fatalf("span err %v", err)
	}
	// SetFileSystem nil is a no-op; non-nil replaces.
	a.SetFileSystem(nil)
	a.SetFileSystem(&fakeFS{})
}

func TestPathParsing(t *testing.T) {
	a := New()
	a.Set("/r/a", "x")
	a.Set("/r/b", "x")
	// predicate: position, last, exists, equal, regexp, self
	a.Set("/r/c/k", "hello")
	if m := a.Match("/r/c[k = 'hello']"); len(m) != 1 {
		t.Fatalf("equal pred %v", m)
	}
	if m := a.Match("/r/c[k =~ 'hel+o']"); len(m) != 1 {
		t.Fatalf("regexp pred %v", m)
	}
	if m := a.Match("/r/c[k]"); len(m) != 1 {
		t.Fatalf("exists pred %v", m)
	}
	if m := a.Match("/r/*[1]"); len(m) != 1 {
		t.Fatalf("pos pred %v", m)
	}
	if m := a.Match("/r/*[last()]"); len(m) != 1 {
		t.Fatalf("last pred %v", m)
	}
	if m := a.Match("/r/c/k[. = 'hello']"); len(m) != 1 {
		t.Fatalf("self value pred %v", m)
	}
	// parent and self axes
	if m := a.Match("/r/a/.."); len(m) != 1 || m[0] != "/r" {
		t.Fatalf("parent axis %v", m)
	}
	if m := a.Match("/r/a/."); len(m) != 1 || m[0] != "/r/a" {
		t.Fatalf("self axis %v", m)
	}
	// parent of root -> nothing
	if m := a.Match("/.."); len(m) != 0 {
		t.Fatalf("root parent %v", m)
	}
	// equal/match against valueless node -> no match
	a.Insert("/r/a", "empty", false)
	if m := a.Match("/r[empty = 'x']"); len(m) != 0 {
		t.Fatalf("valueless equal %v", m)
	}
	if m := a.Match("/r[empty =~ 'x']"); len(m) != 0 {
		t.Fatalf("valueless regexp %v", m)
	}
	// multi-segment subpath predicate and wildcard subpath
	if m := a.Match("/r[c/k = 'hello']"); len(m) != 1 {
		t.Fatalf("nested subpath %v", m)
	}
	if m := a.Match("/r/c[* = 'hello']"); len(m) != 1 {
		t.Fatalf("wildcard subpath %v", m)
	}
	// Match on root
	if m := a.Match("/"); len(m) != 1 || m[0] != "/" {
		t.Fatalf("root match %v", m)
	}
}

func TestPathErrors(t *testing.T) {
	a := New()
	cases := []string{
		"",               // empty path
		"/foo[]",         // empty predicate
		"/foo[0]",        // position out of range
		"/foo[x =~ '(']", // bad regexp
		"/foo[1]bar",     // malformed predicate
		"/foo[1",         // unterminated predicate
		"/[1]",           // empty name test
	}
	for _, c := range cases {
		if _, err := a.eval(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
	// splitOp with quotes and without on equal
	a.Set("/q/k", "v")
	if m := a.Match("/q[k = 'v']"); len(m) != 1 {
		t.Fatalf("quoted equal %v", m)
	}
	if m := a.Match("/q[k = v]"); len(m) != 1 {
		t.Fatalf("unquoted equal %v", m)
	}
}

func TestLensRegistry(t *testing.T) {
	if _, ok := LensByName("Hosts"); !ok {
		t.Fatal("Hosts must be registered")
	}
	if _, ok := LensByName("Nope"); ok {
		t.Fatal("Nope must not exist")
	}
	names := LensNames()
	if len(names) < 4 {
		t.Fatalf("names %v", names)
	}
	// duplicate registration panics
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate register must panic")
		}
	}()
	Register("Hosts", hostsLens{})
}

func TestTextStoreRetrieve(t *testing.T) {
	a := New()
	lens, _ := LensByName("Hosts")
	if err := a.TextStore(lens, "/files/etc/hosts", "127.0.0.1 localhost\n"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Get("/files/etc/hosts/1/canonical"); v != "localhost" {
		t.Fatalf("stored %q", v)
	}
	// store again replaces children
	if err := a.TextStore(lens, "/files/etc/hosts", "10.0.0.1 h\n"); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.Get("/files/etc/hosts/1/ipaddr"); v != "10.0.0.1" {
		t.Fatalf("replaced %q", v)
	}
	// parse error
	if err := a.TextStore(lens, "/files/x", "bad\n"); err == nil {
		t.Fatal("store parse error")
	}
	// createPath error
	if err := a.TextStore(lens, "/bad[1]/x", "127.0.0.1 h\n"); err == nil {
		t.Fatal("store path error")
	}
	// retrieve via path
	out, err := a.TextRetrieve(lens, "/files/etc/hosts", nil)
	if err != nil || out != "10.0.0.1 h\n" {
		t.Fatalf("retrieve %q %v", out, err)
	}
	// retrieve via explicit node
	node := a.Root().firstChild("files").firstChild("etc").firstChild("hosts")
	if out, err := a.TextRetrieve(lens, "", node); err != nil || out != "10.0.0.1 h\n" {
		t.Fatalf("retrieve node %q %v", out, err)
	}
	// retrieve path eval error
	if _, err := a.TextRetrieve(lens, "/[1]", nil); err == nil {
		t.Fatal("retrieve eval error")
	}
	// retrieve path not single
	if _, err := a.TextRetrieve(lens, "/nope", nil); err == nil {
		t.Fatal("retrieve zero match")
	}
	// retrieve build error (hand-built bad tree)
	bad := newNode("f")
	bad.appendChild(newNode("1")) // entry with no ipaddr/canonical
	if _, err := a.TextRetrieve(lens, "", bad); err == nil {
		t.Fatal("retrieve build error")
	}
}

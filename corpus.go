// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-augeas/augeas/internal/interp"
)

// corpusFS embeds the upstream Augeas lens corpus (LGPL; see
// lenses/dist/NOTICE) so the interpreter is fully self-contained.
//
//go:embed lenses/dist/*.aug lenses/dist/tests/*.aug
var corpusFS embed.FS

// corpusSource resolves a module base name to its embedded .aug source.
func corpusSource() interp.Source {
	return func(base string) (string, bool) {
		for _, p := range []string{
			"lenses/dist/" + base + ".aug",
			"lenses/dist/tests/" + base + ".aug",
		} {
			b, err := corpusFS.ReadFile(p)
			if err == nil {
				return string(b), true
			}
		}
		return "", false
	}
}

// Engine interprets the embedded lens corpus. Create one with [NewEngine].
type Engine struct {
	i interface {
		LensValue(module, binding string) (*interp.Lens, error)
		Autoload(module string) (*interp.Lens, []interp.Filter, error)
	}
}

// NewEngine returns an Engine backed by the embedded lens corpus.
func NewEngine() *Engine {
	return &Engine{i: interp.New(corpusSource())}
}

// interpLens adapts a compiled interpreter lens to the [Lens] interface. Only
// the get direction (Parse) is implemented; put (Build) is not yet supported.
type interpLens struct {
	lens *interp.Lens
	name string
}

// Parse turns text into a synthetic parent node whose children are the parsed
// entries, matching the [Lens] contract.
func (l *interpLens) Parse(text string) (*Node, error) {
	forest, err := interp.Get(l.lens, text)
	if err != nil {
		return nil, err
	}
	root := &Node{Label: "/"}
	for _, t := range forest {
		root.appendChild(fromInterpTree(t))
	}
	return root, nil
}

// Build is not implemented: the put (tree->text) direction of interpreted
// lenses is a documented deferred feature.
func (l *interpLens) Build(root *Node) (string, error) {
	return "", fmt.Errorf("augeas: put/Build is not implemented for interpreted lens %q", l.name)
}

func fromInterpTree(t *interp.Tree) *Node {
	n := &Node{}
	if t.Label != nil {
		n.Label = *t.Label
	}
	if t.Value != nil {
		v := *t.Value
		n.Value = &v
	}
	for _, c := range t.Children {
		n.appendChild(fromInterpTree(c))
	}
	return n
}

// Lens returns the [Lens] bound to binding in the named module of the corpus,
// e.g. Lens("Hosts", "lns").
func (e *Engine) Lens(module, binding string) (Lens, error) {
	l, err := e.i.LensValue(module, binding)
	if err != nil {
		return nil, err
	}
	return &interpLens{lens: l, name: module + "." + binding}, nil
}

var moduleNameRe = regexp.MustCompile(`(?m)^module\s+(\w+)`)

// fileLens finds the corpus module whose autoload transform includes path and
// returns its lens. It mirrors the /augeas/load mechanism of real Augeas.
func (e *Engine) fileLens(p string) (Lens, string, error) {
	entries, err := corpusFS.ReadDir("lenses/dist")
	if err != nil {
		return nil, "", err
	}
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".aug") {
			continue
		}
		src, err := corpusFS.ReadFile("lenses/dist/" + name)
		if err != nil {
			continue
		}
		m := moduleNameRe.FindSubmatch(src)
		if m == nil {
			continue
		}
		module := string(m[1])
		lens, filters, err := e.i.Autoload(module)
		if err != nil {
			continue
		}
		if filterMatches(filters, p) {
			return &interpLens{lens: lens, name: module}, module, nil
		}
	}
	return nil, "", fmt.Errorf("no lens found for %q", p)
}

// filterMatches reports whether path is selected by the include/exclude globs.
func filterMatches(filters []interp.Filter, p string) bool {
	included := false
	for _, f := range filters {
		if f.Include && globMatch(f.Glob, p) {
			included = true
		}
	}
	if !included {
		return false
	}
	for _, f := range filters {
		if !f.Include && globMatch(f.Glob, p) {
			return false
		}
	}
	return true
}

func globMatch(glob, p string) bool {
	ok, err := path.Match(glob, p)
	if err == nil && ok {
		return true
	}
	// Augeas globs may use brace/recursive forms; fall back to base match.
	if b, err := path.Match(glob, filepath.ToSlash(p)); err == nil && b {
		return true
	}
	return false
}

// LoadFile reads path through the filesystem seam, selects the matching lens
// from the corpus autoload filters, and stores the parsed tree under
// /files/<path>. It is the interpreted-corpus analogue of Augeas' aug_load.
func (a *Augeas) LoadFile(path string) error {
	lens, _, err := a.engine().fileLens(path)
	if err != nil {
		return a.fail(err)
	}
	data, err := a.fs.ReadFile(path)
	if err != nil {
		return a.fail(err)
	}
	parsed, err := lens.Parse(string(data))
	if err != nil {
		return a.fail(err)
	}
	dst, err := a.createFrom(a.root, "files"+ensureSlash(path))
	if err != nil {
		return a.fail(err)
	}
	dst.Children = nil
	for _, c := range parsed.Children {
		dst.appendChild(c)
	}
	return nil
}

func ensureSlash(p string) string {
	if strings.HasPrefix(p, "/") {
		return p
	}
	return "/" + p
}

// engine lazily creates the Engine used by LoadFile.
func (a *Augeas) engine() *Engine {
	if a.eng == nil {
		a.eng = NewEngine()
	}
	return a.eng
}

var _ fs.FS = corpusFS

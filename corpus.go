// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-augeas/augeas authors

package augeas

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/go-augeas/augeas/internal/interp"
)

// corpusFS embeds the upstream Augeas 1.14.1 lens corpus (LGPL; see
// lenses/dist/NOTICE) so the interpreter is fully self-contained. This tree is
// a verbatim mirror of the upstream distribution and is never edited.
//
//go:embed lenses/dist/*.aug lenses/dist/tests/*.aug
var corpusFS embed.FS

// contribFS embeds go-augeas-original lenses that go beyond the upstream
// 1.14.1 corpus (currently Wireguard and Rclone). They are kept in a separate
// embed and directory (lenses/contrib; see lenses/contrib/NOTICE) so they are
// never confused with the verbatim upstream mirror. They carry the same LGPL
// v2+ header as the corpus, not this package's BSD-3 license.
//
//go:embed lenses/contrib/*.aug
var contribFS embed.FS

// corpusSource resolves a module base name to its embedded .aug source, trying
// the verbatim upstream mirror first and then the go-augeas-original contrib
// lenses. Contrib lenses resolve their Util/IniFile/Sep/Rx imports from the
// dist corpus through this same resolver.
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
		if b, err := contribFS.ReadFile("lenses/contrib/" + base + ".aug"); err == nil {
			return string(b), true
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

// caddyfileHeredocRe matches a Caddyfile heredoc opener: an argument token
// `<<MARKER` (optionally preceded by whitespace) whose bareword marker ends the
// line, per Caddy's heredoc syntax. The closing token repeats the opening word,
// a context-free / back-reference construct that a regular Augeas lens cannot
// match, so Caddyfile.lns would SILENTLY MISPARSE such a file (each body line
// becomes a bogus sibling directive). We refuse rather than corrupt.
var caddyfileHeredocRe = regexp.MustCompile(`(?m)(^|[ \t])<<[A-Za-z_][A-Za-z0-9_]*[ \t]*\r?$`)

// checkCaddyfileHeredoc rejects input that opens a heredoc, which Caddyfile.lns
// cannot model (see caddyfileHeredocRe). It is enforced at this API boundary
// (the entry for both Engine.Lens(...).Parse and LoadFile) rather than in the
// interpreter, so the lens' own inline boundary test can still pin the raw
// misparse. Returns nil for any input without a heredoc opener.
func checkCaddyfileHeredoc(text string) error {
	if caddyfileHeredocRe.MatchString(text) {
		return fmt.Errorf("augeas: Caddyfile heredocs unsupported by Caddyfile.lns (`<<MARKER` here-documents cannot be modelled by a regular lens; refusing to avoid silent misparse)")
	}
	return nil
}

// isCaddyfileLens reports whether this lens is the go-augeas-original Caddyfile
// lens, whose name is either "Caddyfile" (autoload) or "Caddyfile.lns" (direct
// binding).
func (l *interpLens) isCaddyfileLens() bool {
	return l.name == "Caddyfile" || strings.HasPrefix(l.name, "Caddyfile.")
}

// isNftablesLens reports whether this lens is the go-augeas-original Nftables
// lens, whose name is either "Nftables" (autoload) or "Nftables.lns" (direct
// binding).
func (l *interpLens) isNftablesLens() bool {
	return l.name == "Nftables" || strings.HasPrefix(l.name, "Nftables.")
}

// checkNftablesLineWrap rejects an nftables ruleset in which a brace group is
// wrapped across more than one physical line, which Nftables.lns cannot model.
// The lens reads each physical line on its own and tells a block opener
// (a line whose last non-blank character is "{") apart from a rule that merely
// contains an inline anonymous set (`{ 22, 80 }`, closed on the same line). An
// element/value list that nft wraps -- `elements = { a,` on one line, `b }` on
// the next -- would therefore SILENTLY MISPARSE into two bogus sibling "rule"
// nodes (a get succeeds with a wrong tree). We refuse rather than corrupt, at
// this API boundary, so the lens' own inline boundary test can still pin the
// raw misparse.
//
// A line is accepted when, after stripping a trailing "#" comment, it is: blank,
// a lone block closer "}", brace-balanced (every inline `{...}` closed on the
// line), or a block opener (exactly one unmatched "{" and it is the last
// non-blank character). Anything else -- an inline "{" left open mid-line, a
// stray "}" continuing a previous line, or more than one unmatched "{" -- is a
// wrap and is rejected.
func checkNftablesLineWrap(text string) error {
	for n, raw := range strings.Split(text, "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i] // drop trailing comment
		}
		line = strings.TrimRight(line, " \t\r")
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || trimmed == "}" {
			continue // blank / comment-only line, or a block closer
		}
		depth := 0
		neg := false
		for _, c := range line {
			switch c {
			case '{':
				depth++
			case '}':
				depth--
				if depth < 0 {
					neg = true // a "}" with no matching "{" on this line
				}
			}
		}
		if neg {
			return fmt.Errorf("augeas: nftables line %d continues a brace group from a previous line; Nftables.lns cannot model brace groups wrapped across physical lines (refusing to avoid silent misparse)", n+1)
		}
		switch {
		case depth == 0:
			// every inline brace group closed on this line: fine
		case depth == 1 && strings.HasSuffix(line, "{"):
			// a block opener: the sole unmatched "{" ends the line
		default:
			return fmt.Errorf("augeas: nftables line %d opens a brace group it does not close on the same line; Nftables.lns cannot model brace groups wrapped across physical lines (refusing to avoid silent misparse)", n+1)
		}
	}
	return nil
}

// Parse turns text into a synthetic parent node whose children are the parsed
// entries, matching the [Lens] contract.
func (l *interpLens) Parse(text string) (*Node, error) {
	if l.isCaddyfileLens() {
		if err := checkCaddyfileHeredoc(text); err != nil {
			return nil, err
		}
	}
	if l.isNftablesLens() {
		if err := checkNftablesLineWrap(text); err != nil {
			return nil, err
		}
	}
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

// moduleNamesIn returns the declared module name of every .aug lens directly
// under dir in fsys. Non-lens directory entries (such as the tests subdir) are
// skipped.
func moduleNamesIn(fsys embed.FS, dir string) []string {
	var names []string
	entries, _ := fsys.ReadDir(dir)
	for _, ent := range entries {
		if !strings.HasSuffix(ent.Name(), ".aug") {
			continue
		}
		src, _ := fsys.ReadFile(dir + "/" + ent.Name())
		if m := moduleNameRe.FindSubmatch(src); m != nil {
			names = append(names, string(m[1]))
		}
	}
	return names
}

// corpusModuleNames returns the declared module name of every embedded lens,
// computed once: the verbatim upstream dist corpus first, then the
// go-augeas-original contrib lenses, so both are autoloadable by LoadFile.
var corpusModuleNames = sync.OnceValue(func() []string {
	names := moduleNamesIn(corpusFS, "lenses/dist")
	names = append(names, moduleNamesIn(contribFS, "lenses/contrib")...)
	return names
})

// fileLens finds the corpus module whose autoload transform includes path and
// returns its lens. It mirrors the /augeas/load mechanism of real Augeas.
func (e *Engine) fileLens(p string) (Lens, string, error) {
	for _, module := range corpusModuleNames() {
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
	return err == nil && ok
}

// LoadFile reads path through the filesystem seam, selects the matching lens
// from the corpus autoload filters, and stores the parsed tree under
// /files/<path>. It is the interpreted-corpus analogue of Augeas' aug_load.
func (a *Augeas) LoadFile(path string) error {
	if err := a.loadFileInto(path); err != nil {
		return a.fail(err)
	}
	return nil
}

func (a *Augeas) loadFileInto(path string) error {
	lens, _, err := a.engine().fileLens(path)
	if err != nil {
		return err
	}
	data, err := a.fs.ReadFile(path)
	if err != nil {
		return err
	}
	parsed, err := lens.Parse(string(data))
	if err != nil {
		return err
	}
	dst, err := a.createFrom(a.root, "files"+ensureSlash(path))
	if err != nil {
		return err
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

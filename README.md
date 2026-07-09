# go-augeas/augeas

Pure-Go ([CGO=0], stdlib-only) implementation of the core of
[Augeas](https://augeas.net/), the configuration-editing library from the
Puppet ecosystem. It models configuration files as an ordered tree, exposes an
XPath-like path language to query and edit that tree, and uses *lenses* to
translate between the tree and concrete file syntax.

```go
a := augeas.New()
lens, _ := augeas.LensByName("Hosts")
_ = a.TextStore(lens, "/files/etc/hosts", "127.0.0.1 localhost\n")

v, _ := a.Get("/files/etc/hosts/1/canonical") // "localhost"
_ = a.Set("/files/etc/hosts/1/alias", "loopback")
out, _ := a.TextRetrieve(lens, "/files/etc/hosts", nil)
```

The adapter [`github.com/go-ruby-augeas/augeas`](https://github.com/go-ruby-augeas/augeas)
layers the `ruby-augeas` gem API over this engine, mirroring the
`go-facter`/`go-ruby-facter` engine+adapter split.

## Core API

`New` returns an empty tree. On `*Augeas`:

`Get`, `Exists`, `Set`, `SetMultiple`, `Insert`, `Remove`, `Move`, `Match`,
`Label`, `DefineVariable`, `DefineNode`, `TextStore`, `TextRetrieve`, `Load`,
`Save`, `Span`, `Error`, `Root`, `SetFileSystem`.

`Load`/`Save` go through the injectable `FileSystem` seam (`ReadFile`,
`WriteFile`, `Glob`); the default seam is the real OS. Load failures are
recorded under `/augeas/files/<name>/error` (reachable via `/augeas//error`).

## Supported path constructs

- absolute paths (`/files/etc/hosts`) and relative paths (resolved from the root)
- `*` — any child (name wildcard)
- `//` — descendant axis (any depth)
- `.` — self, `..` — parent
- positional predicates `[n]` (1-based) and `[last()]`
- value predicates `[subpath = 'value']` and `[. = 'value']`
- regexp predicates `[subpath =~ 'regexp']` (Go `regexp` / RE2 syntax)
- existence predicates `[subpath]`
- union `|`
- variables: `DefineVariable`/`DefineNode` bind a name usable as `$name` at the
  head of a path (`$name/child`)

Subpaths inside predicates are simple relative paths (labels, `*`, `.`,
slash-separated); they do not themselves take predicates.

## Built-in lenses

Registered by name, each round-trips a canonical, newline-terminated text form:

| Name | File | Notes |
|------|------|-------|
| `Hosts` | `/etc/hosts` | numbered entries with `ipaddr`, `canonical`, `alias`, inline `#comment` |
| `Fstab` | `/etc/fstab` | `spec`/`file`/`vfstype`/`opt`*/`dump`/`passno`; dump & passno optional |
| `Shellvars`, `Simplevars` | `KEY=value` | one key per node, `#` comments |
| `Ini`, `Keyvalue` | INI | `[section]` grouping, top-level keys, `#`/`;` comments |

## Deferred (not yet implemented — honest scope)

This is a faithful **starter** engine, not a drop-in replacement for upstream
Augeas. Known gaps:

- **Lens catalogue**: upstream ships ~200 lenses; this repo ships 4. Additional
  lenses are a follow-on.
- **The Augeas lens DSL** (`.aug` regular-language definitions) is *not*
  interpreted; lenses here are hand-written Go implementing the `Lens`
  interface.
- **Span tracking**: `Span` always returns `ErrSpanUnsupported` — byte offsets
  are not retained when a lens parses text.
- **Path language**: node-set functions beyond `last()` (e.g. `count()`,
  `position()` arithmetic), the `label`/`glob` operators, `re.sub`-style
  transforms, and full XPath axes are not implemented.
- **Non-canonical whitespace/blank-line preservation**: lenses normalise
  whitespace to a documented canonical form and drop blank lines; byte-exact
  round-trip is guaranteed only for canonical input.
- **`aug_load` autodetection** via `/augeas/load` transforms is not modelled;
  `Load` takes an explicit lens, glob and mount point.

## License

BSD-3-Clause. See [LICENSE](LICENSE).

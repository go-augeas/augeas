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

## Embedded lens corpus and go-augeas-original lenses

Beyond the hand-written Go lenses above, the repository embeds the upstream
Augeas **1.14.1** lens corpus as verbatim `.aug` DSL sources under
`lenses/dist/` (LGPL v2+, see `lenses/dist/NOTICE`) and interprets them with a
pure-Go engine (`NewEngine().Lens("Hosts", "lns")`). That corpus is a faithful
mirror and is never edited; it is gated in CI by Augeas' own `test` assertions
(`TestCorpus`: get 1533/1533, put 258/258).

Alongside it, `lenses/contrib/` holds **go-augeas-original lenses** — lenses we
wrote for formats upstream 1.14.1 does not cover. They live in a separate embed
and directory so they are never confused with the upstream mirror, but they
load and run like any other lens (importing `Util`/`IniFile`/`Sep`/`Rx` from the
dist corpus) and are CI-gated the same way by `TestContribCorpus`. They keep the
corpus **LGPL v2+** license, not this repo's BSD-3 (see `lenses/contrib/NOTICE`).

| Lens | File | Coverage | Notes |
|------|------|----------|-------|
| `Wireguard.lns` | `/etc/wireguard/*.conf` | 4/4 (2 get, 2 put) | `[Interface]`/`[Peer]` INI; verbatim to-EOL values so base64 keys ending in `=`, comma-separated `AllowedIPs`, and `PostUp`/`PostDown` shell hooks with `;` round-trip |
| `Rclone.lns` | `rclone.conf` | 3/3 (1 get, 2 put) | one `[remote]` section each; verbatim to-EOL values so OAuth JSON token blobs survive. Caveat: a bare `key =` **empty value** does not round-trip through INI separator defaults (rclone normally omits empty options) |

As a related differentiator, go-augeas' interpreted **`Toml.lns` put works**
(its embedded put test passes in `TestCorpus`) where upstream Augeas' Toml
save/put is buggy (hercules-team/augeas issues #715, #699).

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

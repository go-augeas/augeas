# go-augeas/augeas

Pure-Go ([CGO=0], stdlib-only) implementation of the core of
[Augeas](https://augeas.net/), the configuration-editing library from the
Puppet ecosystem. It models configuration files as an ordered tree, exposes an
XPath-like path language to query and edit that tree, and uses *lenses* to
translate between the tree and concrete file syntax.

```go
a := augeas.New()
lens, _ := augeas.NewEngine().Lens("Hosts", "lns") // interpreted from the embedded .aug corpus
_ = a.TextStore(lens, "/files/etc/hosts", "127.0.0.1 localhost\n")

v, _ := a.Get("/files/etc/hosts/1/canonical") // "localhost"
_ = a.Set("/files/etc/hosts/1/alias", "loopback")
// NOTE: TextRetrieve/Save need Lens.Build, which is not yet wired up for
// interpreted lenses (see "Deferred" below) — this call returns an error today.
_, err := a.TextRetrieve(lens, "/files/etc/hosts", nil)
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

## The `Lens` seam

`Register`/`LensByName` are a pluggable extension point: a hand-written Go type
implementing `Lens` (`Parse`/`Build`) can be registered under a name and looked
up later, for a caller who wants a lens with full get **and** put round-trip
control outside the interpreted corpus below. No lens ships pre-registered —
see the next section for what actually ships.

## Embedded lens corpus and go-augeas-original lenses

This is what actually ships: the repository embeds the upstream Augeas
**1.14.1** lens corpus — all 232 modules — as verbatim `.aug` DSL sources
under `lenses/dist/` (LGPL v2+, see `lenses/dist/NOTICE`) and interprets them
with a from-scratch, pure-Go `.aug` DSL interpreter (`internal/interp`),
reachable as a `Lens` via `NewEngine().Lens("Hosts", "lns")`. That corpus is a
faithful mirror and is never edited; it is gated in CI by Augeas' own `test`
assertions (`TestCorpus`: get 1533/1533, put 258/258 — 1791/1791 overall,
100%). **Those get/put counts validate the interpreter itself** (via the
package-internal `LnsGet`/`LnsPut`); the public `Engine.Lens(...)` adapter
currently only wires up `Parse` (get) — see "Deferred" below for why `Build`
(put), and therefore `TextRetrieve`/`Save`, do not yet work on lenses obtained
this way.

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
| `Caddyfile.lns` | `/etc/caddy/Caddyfile`, `/etc/caddy/conf.d/*` | 13/13 (10 get, 3 put) | native Caddyfile via one recursive subtree with an optional inner block (no per-keyword allow-list). Round-trips: nested directive blocks ✅, `@name` matchers ✅, `(snippet)` definitions ✅, leading global-options block (`@global`) ✅, `{placeholder}` tokens as opaque args ✅, quoted args ✅, comments ✅ |
| `Nftables.lns` | `/etc/nftables.conf`, `/etc/nftables/*.nft`, `/etc/sysconfig/nftables.conf` | 11/11 (8 get, 3 put) | native nftables ruleset via the same recursive block pattern (`table` → `chain`/`set`/`map`/`flowtable` → rule lines). Round-trips: `table <family> <name>` blocks ✅, chain/set/map/flowtable blocks ✅, base-chain `type … hook … priority …; policy …;` line and rule lines as ordered verbatim `rule` nodes ✅, inline **anonymous sets** `{ 22, 80 }` and **verdict maps** `vmap { … }` as opaque rule text ✅, `define NAME = value` (structured) ✅, `include "…"` (structured) ✅, comments ✅ |
| `Unbound.lns` | `/etc/unbound/unbound.conf`, `/etc/unbound/unbound.conf.d/*.conf` | 7/7 (3 get, 4 put) | Unbound resolver: colon-terminated clause headers (`server:`, `forward-zone:`, `remote-control:`, …) over indented `key: value` option lines, top-level `include:` directives and `#` comments. Repeated keys (`interface:`, `access-control:`, `local-data:`, `forward-addr:`, …) are an **ordered list of nodes**, so their order + multiplicity survive edit and append (not a map that collapses dups). Verbatim to-EOL values so quoted strings with spaces, CIDR+action, and `IP@port` round-trip. Indentation discriminates clause bodies from column-0 directives |

**`Caddyfile.lns` boundaries (enforced / documented, never silently lossy):**

- **Heredocs are rejected, not misparsed.** A Caddyfile using a `<<MARKER`
  here-document cannot be modelled by a regular lens — the closing token
  repeats the opening word, a context-free construct Augeas regexps cannot
  match. Rather than silently corrupt such a file (each body line would become
  a bogus sibling directive), the go-augeas API layer **refuses** it:
  `Engine.Lens("Caddyfile", "lns").Parse(...)` and `LoadFile` return
  `Caddyfile heredocs unsupported by Caddyfile.lns` when the input opens a
  heredoc. (The lens' own inline boundary test still pins the raw misparse at
  the interpreter level, so the limitation stays visible.)
- **Empty blocks `dir { }` are excluded.** A block body is deliberately
  non-nullable so a bare directive `dir` never gains braces on put; the price
  is that a literal empty brace pair fails `get`. Empty blocks are practically
  never written in real Caddyfiles (a site or directive block always carries
  directives).

**`Nftables.lns` boundaries (enforced / documented, never silently lossy):**

- **Rule expressions are stored verbatim, not fully parsed.** A base-chain
  setting line (`type filter hook input priority 0; policy drop;`) and every
  rule (`ip saddr @blocklist tcp dport { 22, 80 } accept`) become ordered
  `rule` nodes holding the exact line text. The block structure `table →
  chain/set/map/flowtable` is parsed (family, name), but the packet-match /
  verdict grammar inside a rule — including inline **anonymous sets**
  `{ 22, 80 }`, **verdict maps** `vmap { … }`, ranges and concatenations — is
  kept as opaque text. This is by design: it round-trips faithfully without a
  full nft expression grammar. `define`/`include` get light structure.
- **Line-wrapped brace groups are rejected, not misparsed.** `nft list ruleset`
  may wrap a long list across physical lines (`elements = { a,` on one line,
  `b }` on the next). The lens reads each physical line on its own — a line
  ending in `{` is a block opener, a line with an inline `{ … }` closed on the
  same line is a rule — so a wrapped group would **silently misparse** into two
  bogus sibling `rule` nodes. Rather than corrupt such a file, the go-augeas API
  layer **refuses** it: `Engine.Lens("Nftables", "lns").Parse(...)` and
  `LoadFile` return `nftables line N … brace groups wrapped across physical
  lines`. (The lens' own inline boundary test still pins the raw misparse at the
  interpreter level, so the limitation stays visible.) Single-line lists
  (`elements = { a, b }`) are unaffected.

**`Unbound.lns` boundaries (documented, never silently lossy):**

- **No silent-misparse footgun, so no API guard.** Unlike a Caddyfile heredoc,
  unbound.conf has no context-free / back-reference construct. Any input the
  lens does not model (an option line with no enclosing clause, a bare
  column-0 directive that is neither a known clause header nor `include:`, an
  unknown clause keyword) fails `get` **loudly** instead of being silently
  restructured. Two inline `get ... = *` tests pin this, so no `Parse`/`LoadFile`
  rejection guard is required (contrast `Caddyfile.lns`).
- **Inline trailing comments are not split.** A value is stored verbatim to the
  end of the line, so `verbosity: 1  # note` keeps `1  # note` as the value
  (it round-trips faithfully; the comment is simply not a separate node), the
  same verbatim-value choice as `Wireguard.lns`/`Rclone.lns`.
- **Clause bodies must be indented.** The lens uses indentation to tell a
  clause option line (indented) from a top-level `include:`/clause header
  (column 0) — the universal real-world convention; a de-indented option line
  is rejected rather than misfiled.

As a related differentiator, go-augeas' interpreted **`Toml.lns` put works**
(its embedded put test passes in `TestCorpus`) where upstream Augeas' Toml
save/put is buggy (hercules-team/augeas issues #715, #699).

## Deferred (not yet implemented — honest scope)

This is a faithful engine, not a drop-in replacement for upstream Augeas.
Known gaps:

- **Public put (`Build`/`TextRetrieve`/`Save`) for interpreted lenses.** The
  `.aug` DSL interpreter itself implements both directions and is verified
  correct — `TestCorpus`/`TestContribCorpus` exercise the corpus's own inline
  `put` assertions at 100% — but the `Lens` adapter `Engine.Lens(...)` returns
  only wires up `Parse`; its `Build` always returns an error. Until that
  adapter is finished, round-tripping a file through the embedded corpus (or
  contrib lenses) is read-only via the public API; a caller who needs full
  get+put today must implement `Lens` by hand and `Register` it (see "The
  `Lens` seam" above).
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

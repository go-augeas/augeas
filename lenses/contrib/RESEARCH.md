# Augeas lens gap analysis & candidate contributions

**Date: 2026-07-11**

Research question: *which useful config-file formats lack an Augeas lens today, and
which can we contribute upstream?* Then draft + prove the top candidates against our
own pure-Go `go-augeas/augeas` interpreter.

All claims below are evidence-based: the "exists?" column was checked against the actual
`lenses/*.aug` list of `hercules-team/augeas` (232 lens files, HEAD `22d715e8`, last
commit 2026-04-08 — upstream is alive but slow: 1.14.1 was the last tagged release,
2023-06-27). The two drafted lenses were run through `go-augeas` and **pass get and put**
(output at the bottom).

> Note: no upstream PR was opened. This is research + local proof; the user decides on
> submission. Drafted lenses carry an **LGPL v2+** header to match the Augeas corpus (a
> separate license from go-augeas' BSD-3 code — not relicensed).

---

## 1. What already exists (surprises)

Several "obvious gaps" are in fact **already covered** — verified against the lens list,
not assumed:

| Format | Reality |
| --- | --- |
| **TOML** | `Toml.lns` exists (`lenses/toml.aug`, Raphaël Pinson). Handles typed values, dotted/`[[array]]` tables, inline tables. So `containerd config.toml`, `registries.conf`, `pyproject.toml`, `rustfmt`, `netdata` are *already* addressable. Known upstream bugs: nested arrays are only 1–2 dimensional (issue #715 comment) and put/save has issues (issue #715, #699). **go-augeas passes all embedded toml get+put tests.** |
| **YAML** | `YAML` lens exists BUT its own header says *"Only valid for the following subset"* — anchors + 2-space nesting of scalar maps only. **Not** a general YAML parser. Netplan/cloud-init/kubeconfig/compose/prometheus.yml are effectively **uncovered**. |
| **systemd `.network`/`.netdev`/`.link`** | Covered. `Systemd.lns` is generic INI and its filter already globs `/etc/systemd/network/*`, `/lib/systemd/network/*`. No new lens needed. |
| **containers `registries.conf`** | TOML → covered by `Toml.lns`. |
| **step-ca `ca.json`**, **Tailscale state** | JSON → covered by `Json.lns`. |
| **chrony / ntp** | `Chrony`, `Ntp`, `Ntpd` all exist. |
| **iptables** | `Iptables` exists (save-file format). **nftables is distinct and absent.** |
| **PowerDNS `pdns.conf`** | Plain `key=value`; no dedicated lens but trivially handled by `Simplevars`/generic. Low marginal value. |

## 2. Genuine gaps (format → exists? → usage → feasibility)

Feasibility grades reflect the `.aug` language's line-oriented nature. Our own corpus
work confirmed that deeply-nested/recursive structures (`square`/recursive lenses) are
the hard path; INI-like line formats are the sweet spot.

| Format | Lens? | 2026 usage | Feasibility | Reason |
| --- | --- | --- | --- | --- |
| **WireGuard** `wg0.conf` | **No** | Very high (systemd-networkd, wg-quick, every VPN) | **Easy** ✅ drafted | Pure INI: `[Interface]`/`[Peer]` sections, `Key = Value`. Line-oriented, no nesting. |
| **rclone** `rclone.conf` | **No** | High (backups/cloud sync everywhere) | **Easy** ✅ drafted | Pure INI: one `[remote]` section each. Only trap = OAuth JSON token blobs with `"`/`{}` → solved by verbatim to-EOL value store. |
| **nftables** `.nft` | **No** (distinct from Iptables) | High (default firewall on modern distros) | **Hard→Infeasible** | Nested `table { chain { rule } }` braces with recursive expressions. Not line-oriented; `.aug` recursion is the known weak path. A shallow "ruleset-as-lines" lens is possible but not faithful. |
| **Caddyfile** | **No** | High (Caddy web server) | **Hard** | Brace-nested blocks + directives with heterogeneous arg grammar; matchers nest. Recursive; poor `.aug` fit. |
| **CoreDNS Corefile** | **No** | High (k8s cluster DNS) | **Medium→Hard** | Brace blocks `zone { plugin args }`. Two-level nesting is borderline expressible but plugin arg grammar varies wildly. |
| **Traefik** (static/dynamic) | **No** | High | **Infeasible (YAML/TOML file) / N/A** | Config is YAML or TOML → falls under those; the TOML variant is already covered, YAML variant blocked by the YAML wall. |
| **Netplan** | **No** (only YAML subset) | High (Ubuntu default networking) | **Infeasible** | Arbitrary YAML nesting (lists of maps of maps). Not faithfully expressible in `.aug`. |
| **cloud-init**, **kubeconfig**, **docker-compose**, **prometheus.yml**, most k8s | **No** | Very high | **Infeasible** | General YAML — same wall. Honest verdict: `.aug` cannot faithfully model arbitrary block/flow YAML with anchors. |
| **Knot DNS** `knot.conf` | **No** | Medium | **Infeasible** | knot.conf *is* YAML. |
| **Unbound** `unbound.conf` | **No** | High (resolver) | **Medium** | Clause-based (`server:` then indented `key: value`), YAML-ish but shallow & flat-per-clause. Tractable as a specialised lens; repeated keys need list handling. Good future candidate. |
| **restic** | **No** | High (backups) | **N/A** | No config file — driven by env vars + flags. Nothing to lens. |
| **containers `storage.conf`** | **No** | High (podman/CRI-O) | **Easy–Medium** | It's TOML → `Toml.lns` works today; a thin autoload wrapper would just add the file filter. Low marginal value. |
| **containers `policy.json`** | **No** | High | **N/A** | JSON → `Json.lns`. |
| **FRR** (`frr.conf`), **BIRD** (issue #149) | **No** | Medium (routers) | **Hard** | FRR = Cisco-IOS-style indented hierarchy; BIRD = C-like nested blocks. Both recursive. |
| **k3s / containerd** `config.toml` | **No** | High | **Easy** | TOML → covered; only a file-filter wrapper is missing. |

**Also seen in the upstream open-issue wishlist** (165 open issues): syslog-ng (#581),
Makefile (#568), openssl.cnf (#458), bird (#149) — plus bug reports on existing lenses
(TOML save #715/#699, nginx #834, httpd #833, krb5 #828, redis #827).

## 3. The `.aug` feasibility line, stated honestly

- **Tractable (INI/line-oriented):** WireGuard, rclone, Unbound, PowerDNS, storage.conf,
  and any "TOML tool config" via the existing `Toml.lns`. These are the realistic
  contribution surface.
- **Genuinely infeasible to do *faithfully*:** anything whose native format is **general
  YAML** (Netplan, cloud-init, kubeconfig, compose, prometheus, Knot, Traefik-YAML) or
  deeply **recursive-brace** (nftables, Caddyfile, BIRD). Augeas' line-oriented model and
  weak recursion support (our corpus work hit exactly this on `square`/recursive lenses)
  mean a "lens" here would be a lossy subset, not a round-tripping model. We should NOT
  pretend otherwise.

## 4. Drafted + PROVEN lenses

Two Easy, high-value, genuinely-missing formats were drafted with inline
`test … get … = …` and `test … put … after … = …` blocks and run through go-augeas.

### `lenses/contrib/wireguard.aug` — `Wireguard.lns`
- `[Interface]`/`[Peer]` INI; comments and blank lines inside sections; verbatim
  to-EOL values so **base64 keys ending in `=`**, **comma-separated `AllowedIPs`**, and
  **`PostUp`/`PostDown` shell hooks containing `;`** all round-trip.
- Autoload filter: `/etc/wireguard/*.conf`.
- **4/4 tests pass (2 get, 2 put)** incl. a keepalive-mutation put and a shell-hook get.

### `lenses/contrib/rclone.aug` — `Rclone.lns`
- One `[remote]` section per remote; verbatim to-EOL values so **OAuth JSON token blobs**
  (`{"access_token":...}` with `"`, `:`, `{}`) survive.
- Autoload filter: `~/.config/rclone/rclone.conf`, `/etc/rclone.conf`.
- **3/3 tests pass (1 get, 2 put)** incl. a two-value mutation put.
- Known limitation (documented, not shipped broken): a bare `key =` empty value does not
  round-trip cleanly through INI separator defaults; rclone normally omits empty options.

### Proof (go-augeas interpreter, `internal/interp` `TestContribCorpus`)

These lenses were promoted from research drafts into the CI-gated contrib
corpus (`lenses/contrib/`), run in CI alongside `TestCorpus`:

```text
$ GOWORK=off go test ./internal/interp -run TestContribCorpus -v
=== RUN   TestContribCorpus/Wireguard
    contrib_test.go:84: Wireguard: 4/4 tests pass (get=2 put=2)
=== RUN   TestContribCorpus/Rclone
    contrib_test.go:84: Rclone: 3/3 tests pass (get=1 put=2)
--- PASS: TestContribCorpus (0.00s)
    --- PASS: TestContribCorpus/Wireguard (0.00s)
    --- PASS: TestContribCorpus/Rclone (0.00s)
ok  github.com/go-augeas/augeas/internal/interp
```

The harness reuses the exact `runOneTest`/`LnsGet`/`LnsPut` path that validates the
embedded 1.14.1 corpus (get 1533/1533, put 258/258), resolving imports (`Util`,
`IniFile`, `Sep`, `Rx`) from the embedded dist corpus. `TestCorpus` still passes
unchanged.

## 5. Recommended contribution shortlist (for user approval)

Ranked by value × feasibility:

1. **WireGuard** — proven, Easy, ubiquitous, no existing lens, no upstream competitor.
   **Strongest submission.** Shipped as `lenses/contrib/wireguard.aug`.
2. **rclone** — proven, Easy, widely used. Shipped as `lenses/contrib/rclone.aug`.
   (Optionally solve the empty-value case for full fidelity.)
3. **Unbound** — not drafted; Medium but tractable and high-value (resolver). Good next
   target if the user wants a third.
4. **File-filter wrappers around `Toml.lns`** for `containerd`/`k3s config.toml`,
   `storage.conf`, `registries.conf` — near-zero effort, purely additive value.

**Do NOT attempt** faithful lenses for general-YAML formats (Netplan, cloud-init,
kubeconfig, compose, Knot, Traefik-YAML) or recursive-brace formats (nftables, Caddyfile,
BIRD) — `.aug` cannot round-trip them without lossy subsetting. If those are needed, the
right answer is a non-Augeas tool, not a lens.

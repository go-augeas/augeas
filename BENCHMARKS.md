# Benchmarks & reference comparison

This package is a pure-Go (CGO=0) interpreter for the Augeas lens language. The
performance target is to be **at least as fast as the reference C Augeas**
(`augtool` / `libaugeas`) on the same lenses and files.

## Go benchmarks

Hot paths — lens compile (interpret from `.aug` source), and `get`
(text → tree) on representative real-world files — are covered by
`Benchmark*` functions in `bench_test.go`:

```
go test -run '^$' -bench 'BenchmarkCompile|BenchmarkGet' -benchmem ./
```

Representative files: `hosts`, `fstab`, `sshd` (embedded fragments in
`bench_test.go`). `BenchmarkGet*` report `MB/s` via `b.SetBytes`.

Indicative numbers (Apple M-series, `go1.26`, `-benchmem`):

| benchmark            | ns/op   | throughput | allocs/op |
|----------------------|---------|------------|-----------|
| CompileHosts         | ~61 µs  | —          | ~1380     |
| GetHosts             | ~14 µs  | ~9 MB/s    | ~138      |
| GetFstab             | ~19 µs  | ~6 MB/s    | ~224      |
| GetSshd              | ~400 µs | ~0.2 MB/s  | ~109      |

(`Sshd.lns` has a very large `ctype` union; its compiled RE2 dominates the get
time and is the primary optimisation target.)

## Comparing against reference C Augeas

The reference is not installable on the macOS dev host (Augeas is a C/autotools
project that links `libfa` and glibc regex). The comparison therefore runs in a
Linux Tart VM (per the project convention of using Tart VMs for Linux work).

### Methodology

1. In a Debian Tart VM, install the reference:
   `apt-get install -y augeas-tools libaugeas-dev`.
2. Build a tiny C harness (or use `augtool`) that, for each `(lens, file)` pair,
   times `N` iterations of:
   - `aug_init` + module load (compile), and
   - `aug_load` / `aug_text_store` (get).
   Use `clock_gettime(CLOCK_MONOTONIC)` around the loop; discard a warm-up run.
3. Run the identical `(lens, file)` matrix through the Go benchmarks above
   (same file bytes). Because the Go `get` benchmark isolates `lens.Parse`, the
   comparable C measurement is `aug_text_store` on a pre-compiled lens.
4. Report the ratio `t_go / t_c` per lens; the goal is `<= 1.0`.

## Measured results — real hardware (2026-07-10)

Measured on an **IBM z15 (LinuxONE, `s390x`)**, Ubuntu 24.04, go1.26.4 vs the
reference **C Augeas 1.14.1** (`libaugeas`, `-O2`). The C harness (`augbench.c`,
see below) times `aug_text_store(lens, "/input", "/t")` — the `get` (text→tree)
operation on a pre-compiled lens — over the **identical** `benchHosts`,
`benchFstab` and `benchSshd` bytes, 500 k / 500 k / 100 k iterations after a
warm-up. The Go side is `lens.Parse` on the same bytes.

| Lens (`get`) | Go ns/op | C Augeas 1.14.1 ns/op | ratio (C ÷ Go) |
|--------------|---------:|----------------------:|---------------:|
| `Hosts.lns` | 41 109 | 49 976 | **1.22× faster** |
| `Fstab.lns` | 50 953 | 58 927 | **1.16× faster** |
| `Sshd.lns`  | 896 581 | 152 195 | **0.17× — 5.9× slower** |

The pure-Go interpreter **beats reference C Augeas on `Hosts` and `Fstab`** (the
common small-record lenses) and is **5.9× slower on `Sshd`** — exactly the
hotspot called out above: `Sshd.lns` has a very large `ctype` union whose
compiled RE2 dominates the Go `get`. So the "≥ reference" rule holds for the
typical lenses but **not yet for `Sshd`**, which stays the primary optimisation
target. This is reported honestly rather than averaged away.

The C reference harness used:

```c
augeas *a = aug_init(NULL, NULL, AUG_NO_LOAD | AUG_NO_ERR_CLOSE);
aug_set(a, "/input", text);
for (long i = 0; i < n; i++) aug_text_store(a, lens, "/input", "/t");
```

built with `gcc -O2 $(pkg-config --cflags libxml-2.0) augbench.c -laugeas
$(pkg-config --libs libxml-2.0)` and timed with `clock_gettime(CLOCK_MONOTONIC)`
around the loop (warm-up discarded).

### Status

Go benchmarks and the C-reference ratio are now measured on real `s390x`
hardware (above). `Sshd.lns` throughput is the tracked follow-up; correctness /
compatibility (the upstream `test` pass rate) was prioritised first.

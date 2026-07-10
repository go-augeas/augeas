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

### Status

Go benchmarks are landed and reproducible. The Go-vs-C ratio must be measured in
the Tart VM (the C reference does not build on the dev host); the harness above
is the documented procedure. This is tracked as follow-up perf work; correctness
/ compatibility (the upstream `test` pass rate) was prioritised first.

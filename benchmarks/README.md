# Go 1.26 optimization and compatibility report

This change preserves the public API and tested parsing behavior while reducing
avoidable allocations. The minimum supported compiler is now **Go 1.26**, as
requested; compatibility with earlier Go toolchains is intentionally dropped.

The baseline is commit `8d89927eff414537088a6092d53fecf6711c1e75`. Both versions
are compiled with **Go 1.26.8**, on **darwin/arm64, Apple M3 Pro**. The benchmark
harness is identical on both sides. There are ten samples per case, 200 ms per
sample, with `GOMAXPROCS=1`. Execution alternates before/after and after/before
between pairs. Validation and fuzzing finish before timing begins.

## Measurements

| Case | Before ns/op | After ns/op | Time change | B/op before → after | allocs/op before → after |
|---|---:|---:|---:|---:|---:|
| Plain string lookup | 21.47 | 21.66 | +0.88% | 0 → 0 | 0 → 0 |
| Short escaped string | 70.04 | 71.40 | +1.93% | 16 → 16 | 1 → 1 |
| Unicode escape to ASCII | 94.00 | 70.60 | -24.89% | 16 → 0 | 1 → 0 |
| Single-character escape | 63.14 | 61.66 | -2.35% | 0 → 0 | 0 → 0 |
| Long escaped string | 252.30 | 193.70 | -23.23% | 224 → 112 | 2 → 1 |
| Long escaped string from bytes | 289.25 | 230.35 | -20.36% | 448 → 336 | 4 → 3 |
| Mixed-case boolean | 30.53 | 1.93 | -93.67% | 8 → 0 | 1 → 0 |
| Invalid boolean | 39.51 | 1.92 | -95.13% | 56 → 0 | 2 → 0 |
| Dense Unicode escapes | 1,714.50 | 1,715.50 | +0.06% | 896 → 896 | 2 → 2 |
| Projection, 3 elements | 327.65 | 270.10 | -17.56% | 528 → 48 | 2 → 2 |
| Numeric query, 1,000 elements | 66,585.50 | 48,569.50 | -27.06% | 0 → 0 | 0 → 0 |
| Invalid numeric operand, 1,000 elements | 102,161.00 | 48,411.50 | -52.61% | 56000 → 56 | 2000 → 2 |
| Iterator control | 62.43 | 62.30 | -0.21% | 0 → 0 | 0 → 0 |

Statistically significant timing increases in this run are listed explicitly:

- `Lookup/GetBytes/escaped`: **+2.20%** (p=0.015, n=10).
- `Lookup/Get/age`: **+1.62%** (p=0.004, n=10).
- `Lookup/GetBytes/age`: **+1.01%** (p=0.011, n=10).
- `DecodeDensity/invalid`: **+1.21%** (p=0.029, n=10).

These timing increases remain part of the result; the change does not promise
lower latency for every path. The API/behavior checks are separate from timing.

The table reports medians. Raw samples and the complete statistical comparison,
including unchanged controls and any slower cases, are retained in
[before.txt](before.txt), [after.txt](after.txt), and [comparison.txt](comparison.txt).
[environment.txt](environment.txt) records the source and harness SHA-256 hashes.
These are microbenchmarks, not a guarantee for every application workload.

## Implementation

- `Result.Bool` recognizes true spellings directly, avoiding lowercasing and
  discarded parse errors. False spellings and invalid strings still return false.
- Escape decoding handles single-character escapes directly, copies ordinary spans
  in batches, and uses `utf8.AppendRune`. Dense escapes skip the span-copy path.
  Inputs of at most 32 bytes use stack scratch space. Larger outputs can retain
  their exclusively owned decoding buffer without a second copy; that buffer is
  never reused. Empty/single-byte outputs and outputs smaller than half the input
  use a string copy to avoid retaining an oversized buffer.
- Numeric query operands are parsed lazily once per array query. Mixed-type
  comparisons, invalid operands, overflow, NaN, and boolean coercion keep the
  baseline behavior.
- Projection index capacity starts at `min(len(alog), 64)` rather than always 64.
  Element order, nil-versus-empty results, and index values are unchanged. Slice
  capacity is intentionally allowed to change. The existing projection traversal
  is retained rather than combining this change with a larger parser rewrite.
- Internal structs were aligned. The public `Result` field order and offsets are
  unchanged. `GetBytes` still copies results away from mutable input buffers.
- `go fix` from Go 1.26 modernized applicable code, including `any` and range loops.
  New benchmarks use `testing.B.Loop`. See the official
  [Go 1.26 release notes](https://go.dev/doc/go1.26#tools) for the modernizer.
- Ordinary Go comments were removed with a parser-aware process. The tokenizer
  found zero comments across the five repository Go files. No generated Go files
  were present. The original copyright attribution is preserved in `LICENSE`.

## Compatibility verification

- Go 1.26.8: full test suite, `go vet`, race detector, and strict pointer checks passed.
- Go 1.27.1: full test suite passed.
- Baseline differential suite passed, including all 45 public declarations and the
  size, field order, and offsets of `Result`.
- Final differential fuzz run: **4,128,907 executions in 30 seconds**, no mismatch.
- Boolean compatibility fuzzing: **2,880,520 executions**, no mismatch.
- Final standards-based string fuzz run: **1,356,458 executions**, no mismatch.
- `go fix -diff` produced no remaining modernization changes; `git diff --check`
  passed.

The recorded validation and fuzzing output are in [validation.txt](validation.txt).

The differential harness compares all `Result` fields (including floating-point
bits and nil/empty `Indexes`), value conversions, mutable byte-input ownership,
validation, collection materialization, and iteration. It includes valid and
malformed input, queries, projections, modifiers, pipes, duplicate keys, escaped
keys, Unicode, and numeric boundaries. More than 40,000 deterministic comparisons
are followed by mutation fuzzing against the original source.

`betteralign -apply` was reviewed. A subsequent scan has one intentional diagnostic:
`Result` could lower its GC pointer-scan estimate by reordering fields. That change
was reverted to preserve public layout and positional literals. The diagnostic is
not a failing functional test, and no claim is made that layout tuning alone proves
a runtime speedup.

Zero-allocation assertions are part of the normal and race test suites. They are
excluded only from the `checkptr=2` run, because that instrumentation forces
otherwise unnecessary heap escapes. Pointer and behavioral checks still run.

No regression was found in these checks. Finite tests and fuzzing cannot prove
correctness for every possible input. Zero allocation applies to selected paths;
owned strings, returned collections, and synthesized JSON may still allocate.

## Reproduce

Run from the repository root:

```sh
GOTOOLCHAIN=go1.26.8 go test ./...
GOTOOLCHAIN=go1.26.8 go vet ./...
GOTOOLCHAIN=go1.26.8 go test -race ./...
GOTOOLCHAIN=go1.26.8 go test -gcflags=all=-d=checkptr=2 -skip '^TestZeroAllocationPaths$' ./...
GOTOOLCHAIN=go1.26.8 sh scripts/compare-baseline.sh
GOTOOLCHAIN=go1.26.8 sh scripts/compare-baseline.sh -run '^$' -fuzz '^FuzzCompatibility$' -fuzztime=30s -parallel=4
GOTOOLCHAIN=go1.26.8 go test -run '^$' -fuzz '^FuzzBoolCompatibility$' -fuzztime=20s -parallel=4
GOTOOLCHAIN=go1.26.8 go test -run '^$' -fuzz '^FuzzStringDecode$' -fuzztime=20s -parallel=4
GOTOOLCHAIN=go1.26.8 sh scripts/benchmark-baseline.sh
go run golang.org/x/perf/cmd/benchstat@v0.0.0-20260908200009-22c9c6c9d4da -table goos,goarch,cpu -ignore pkg benchmarks/before.txt benchmarks/after.txt
```

`BASELINE_REF`, `BENCH_COUNT`, and `BENCH_TIME` override the benchmark defaults.
The benchmark script accepts an output directory as its first argument. Both
scripts isolate the baseline outside the checkout; neither changes git state.
A failing compatibility run retains its temporary workspace and fuzz reproducer.

The recorded benchmark files predate the module rename to `github.com/ad3n/gjson`.
Their original `pkg:` headers are preserved as historical measurement metadata.
The reproduction command groups by platform and ignores the differing module
labels so future baseline/fork comparisons remain in the same table.

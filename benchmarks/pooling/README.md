# Bounded pooling for @reverse

Only temporary `[]Result` storage used by `@reverse` is pooled. The byte buffer
backing the returned string is never pooled; caller-owned results remain valid
after subsequent calls, concurrent use, and garbage collection.

Input strings of at most 16 KiB are eligible. Returned scratch buffers retain at
most 1,024 Result entries (about 80 KiB on this architecture); larger backing
arrays are discarded and only the small holder is reused. Every live Result slot
is cleared before retention, releasing Raw, Str, and Indexes references. Pool
release is deferred immediately after acquisition. Inputs above 16 KiB use the
original allocation approach. Exact empty containers use constant results.

`sync.Pool` may discard entries during GC. Pool misses still allocate, and the
retention cap applies per buffer, not to the total pool across concurrent calls.
This is not a zero-allocation guarantee. Nonempty output strings require owned
storage, and escaped values can require additional decoding allocations.

## Comparison setup

The before snapshot includes the preceding safety fixes, which were uncommitted
when this task started. It is reconstructed from commit
`130841bf288cd4ac696af7674d3600aeecdc3105` plus [baseline.patch](baseline.patch).
Both versions use the same benchmark file and Go 1.26.8 on Apple M3 Pro, with
`-cpu=1`, six paired samples, and 100 ms per case. Execution order alternates.
All validation processes finished before this benchmark run. Sequential steady
state measurements do not measure cold-start cost or parallel throughput.

Raw samples: [before.txt](before.txt), [after.txt](after.txt).
Full statistics: [comparison.txt](comparison.txt).
Toolchain, parameters, and source hashes: [environment.txt](environment.txt).

## Measured results

| Reverse input | Before allocs/op | After allocs/op | Before B/op | After B/op |
|---|---:|---:|---:|---:|
| Empty array/object | 1 | 0 | 2 | 0 |
| Array, 3 entries | 4 | 1 | 592 | 32 |
| Object, 3 entries | 4 | 1 | 1,216 | 32 |
| Array, 64 entries | 8 | 1 | 12,592 | 704 |
| Object, 64 entries | 8 | 1 | 24,672 | 576 |
| Array, 1,000 entries | 12 | 1 | 190,064 | about 10,280 |
| Object, 1,000 entries | 13 | 13 | about 471.5 KiB | about 471.4 KiB |
| Array, 2,048 entries | 14 | 14 | 496,496 | 496,496 |
| Object, 2,048 entries | 16 | 16 | 1,467,936 | 1,467,936 |

The measured allocation reduction justifies retaining bounded pooling for the
eligible cases. The 1,000-entry object exceeds the scratch retention cap, while
the 2,048-entry fixtures exceed the input-size cutoff and bypass pooling.

Timing in the final run was very noisy (some intervals exceed 900%). No individual
timing difference is statistically significant; the time geomean increased
13.15%. These samples do not establish either a speedup or absence of timing
regressions. The reliable benefit demonstrated here is lower allocation volume.
See the raw statistics rather than treating median timings as a speed guarantee.

## Validation

- Full Go 1.26.8 race suite passed, including concurrent modifier registration,
  parallel pool use, result ownership, forced GC, cleared references, and oversized
  backing-array disposal. Existing zero-allocation lookup assertions still pass.
- Strict checkptr suite passed, with only TestZeroAllocationPaths excluded from
  that instrumented mode because instrumentation affects allocations.
- Full Go 1.27.1 suite, Go 1.26.8 vet, and diff whitespace checks passed.
- Differential API/layout and 40,000+ deterministic behavior comparisons passed
  against commit 130841bf, including the documented syntax suite. The safety fixes
  from the previous task remain in place.
- Betteralign was applied and reviewed. Its Result reorder was reverted; that
  public-layout diagnostic is intentionally retained for API compatibility.

## Reproduce

From the repository root:

```sh
pool_baseline=$(mktemp -d)
for file in gjson.go go.mod go.sum; do
    git show 130841bf288cd4ac696af7674d3600aeecdc3105:"$file" > "$pool_baseline/$file"
done
patch -d "$pool_baseline" -p1 < benchmarks/pooling/baseline.patch
BASELINE_REF=130841bf288cd4ac696af7674d3600aeecdc3105 BASELINE_DIR="$pool_baseline" BENCH_COUNT=6 BENCH_TIME=100ms sh scripts/benchmark-baseline.sh benchmarks/pooling
benchstat -table goos,goarch,cpu -ignore pkg benchmarks/pooling/before.txt benchmarks/pooling/after.txt
```

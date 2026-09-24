# Safety change benchmarks

Baseline: `130841bf288cd4ac696af7674d3600aeecdc3105`.
Both versions use identical benchmark source, Go 1.26.8, Apple M3 Pro,
`-cpu=1`, six paired samples, and 100 ms per benchmark case. Run order alternates;
other validation processes had completed before measurement.

All 37 cases retain exactly the same B/op and allocs/op across all samples.
The four modifier lookup cases remain zero allocation and show no statistically
significant timing difference at benchstat's default threshold:

| Case | Before | After |
|---|---:|---:|
| @this | 22.25 ns | 22.38 ns |
| @this\|v | 40.54 ns | 39.95 ns |
| v.@this | 77.12 ns | 77.94 ns |
| @missing | 27.48 ns | 28.18 ns |

Time geomean: +1.01%. One of 37 cases, Get/unicode, shows +1.78% (p=0.004).
The other cases are not statistically distinguishable in this run. Some timing
intervals are wide; these measurements do not prove absence of all slowdowns.
No concurrent-registration throughput or extreme-depth benchmark is included.
Registration copies the registry and serializes writers; lookup takes no mutex.

See [comparison.txt](comparison.txt) for full statistics,
[before.txt](before.txt) and [after.txt](after.txt) for raw samples, and
[environment.txt](environment.txt) for source hashes and parameters.

```sh
BASELINE_REF=130841bf288cd4ac696af7674d3600aeecdc3105 BENCH_COUNT=6 BENCH_TIME=100ms sh scripts/benchmark-baseline.sh benchmarks/safety
benchstat -table goos,goarch,cpu -ignore pkg benchmarks/safety/before.txt benchmarks/safety/after.txt
```

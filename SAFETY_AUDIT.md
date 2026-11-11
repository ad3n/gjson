# Safety audit

Baseline: `130841bf288cd4ac696af7674d3600aeecdc3105`.
The audit covers the parser, unsafe conversions, shared modifier registry,
callback execution, result metadata, and allocation behavior. It does not prove
absence of every possible panic, race, leak, or workload bottleneck.

## Fixed

- Concurrent `AddModifier`, modifier lookup, and `ModifierExists` previously
  accessed an unsynchronized map. Writers now publish immutable snapshots through
  an atomic pointer; a writer mutex prevents lost updates. Readers take no mutex.
- Callbacks execute outside the writer mutex. A regression test registers and
  invokes a second modifier from inside a callback to check reentrancy.
- A registered nil modifier is no longer invoked. Its name remains registered
  (`ModifierExists` returns true), while execution follows the unknown-modifier
  fallback. This intentionally replaces a nil-function panic.
- `Result.Path` rejects negative, out-of-range, and overflowing metadata before
  slicing. Truncated object-key scans return an empty path. Empty reverse scans
  are guarded. Regression cases cover these previously unsafe inputs.

Public declarations and the Result field layout are preserved. Defined valid
input behavior remains compatible; the panic cases above are deliberate fixes.

## Validation

- Full Go 1.26.8 race suite passed, including simultaneous reads/writes,
  independent concurrent registrations, callback reentrancy, syntax checks,
  byte-input ownership, and existing zero-allocation assertions.
- Public-API fuzzing passed 1,913,860 executions in 30 seconds with two workers.
  The target exercises parsing, lookup, validation, collection conversion,
  iteration, paths, arbitrary integer indexes, and string encoding. Inputs are
  bounded to 4 KiB documents and 256-byte paths; this is not an extreme-depth test.
- Strict `checkptr=2` suite passed. Only the allocation assertion test is excluded
  from that instrumented mode because instrumentation changes allocations.
- Full Go 1.27.1 tests and Go 1.26.8 vet passed.
- Differential API/layout and 40,000+ deterministic behavior comparisons passed
  against the baseline, together with the documented syntax suite.
- Betteralign was run and applied; its public Result reorder was reverted to
  preserve compatibility. The single Result pointer-layout diagnostic remains.

## Usage constraints and remaining risks

- Set `DisableModifiers` and `DisableEscapeHTML` before concurrent use, or provide
  application-level synchronization covering every reader and writer. They remain
  public bool variables for compatibility; arbitrary concurrent assignment cannot
  be made safe internally without changing that API.
- Custom modifiers and iterator callbacks must provide their own synchronization.
  A callback can panic, block, recurse indefinitely, or retain data. Nil iterator
  callbacks remain invalid; panics from user callbacks are not swallowed.
- Do not mutate a byte input concurrently with a call reading it. Results returned
  by GetBytes own their string data after the call, as checked by ownership tests.
- Treat shared Result values and their Indexes slices as immutable while reading
  them concurrently. Exported mutable fields cannot be synchronized internally.
- Get results can retain the source string's backing storage. This is zero-copy
  ownership, not an unreferenced allocation leak. Clone strings when retaining a
  tiny result from a large document if that retention matters to the application.
- Registry entries retain their callbacks and captured values. Replacing a name
  releases the previous registry reference once in-flight readers finish; adding
  unbounded distinct names intentionally grows the registry. Snapshot registration
  costs O(number of registered modifiers), so registration is a configuration
  operation, not a recommended per-request operation.
- Deep nesting, recursive descent (`@dig`), deep flattening, and materialized
  collections can consume large amounts of CPU, stack, and heap. Some operations
  revisit nested input. No new depth/size/output limit is imposed because that
  would reject previously supported inputs. Apply workload limits externally for
  untrusted or extremely large documents. Memory exhaustion remains possible.

No internal resource/goroutine leak was identified by inspection, and no race or
unexpected panic was observed in the tests above. There was no long-duration heap
soak or proof of behavior for unlimited inputs. Benchmark results are stored in
[benchmarks/safety](benchmarks/safety/README.md).

## Performance follow-up (2026-11-11)

The local working tree already contained reordered `Result` and
`arrayOrMapResult` fields before this follow-up. Those edits are preserved;
`TestArrayValues` now compares field values instead of Go's formatted field order.

- `@join` reuses private scratch maps/slices for inputs up to 16 KiB. A released
  scratch object retains at most 256 keys; larger maps/slices are dropped.
  Reusable maps and slices are cleared before returning them to `sync.Pool`.
  This bounds each retained scratch object, not the total size of the pool.
- Joined output is allocated independently, sized from the final keys and values.
  Returned strings never alias pooled storage. Duplicate keys preserve the first
  key spelling/order and the last value, including escaped equivalent keys.
- Empty collection fast paths avoid allocation. Escape-prefixed strings of
  33–1024 bytes use a temporary stack buffer and copy the decoded result into
  owned storage. Small-string and large-input paths remain available.
- No new unsafe conversions or public API declarations were introduced.

Five 100 ms benchmark samples on the same host, run sequentially against the
pre-change snapshot and the updated tree, produced these median measurements:

| Workload | ns/op before → after | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: |
| Join 3 keys | 411.8 → 302.9 | 616 → 32 | 6 → 1 |
| Join 64 keys | 9256 → 5174 | 39024 → 642 | 24 → 1 |
| Join empty array | 61.98 → 28.49 | 8 → 0 | 1 → 0 |
| Decode 128 Unicode escapes | 1667 → 1612 | 896 → 128 | 2 → 1 |
| Invalid escape-prefixed string | 88.21 → 71.85 | 144 → 0 | 1 → 0 |

Pool benchmarks describe steady-state reuse. A cold pool or a GC that discards
pool entries can require additional allocations. Nonempty output still requires
owned memory; zero allocation is not claimed for every API or input.

Validation includes the full suite, race detector, vet, betteralign, strict
`checkptr=2` (excluding allocation-count assertions), pool cleanup/size limits,
retained-output and byte-input ownership, parallel joins, duplicate/escaped keys,
and decoder size boundaries. A temporary differential fuzz harness compared
`@join`, preserve mode, `@flatten`, `@keys`, `@values`, and `@this` against the
pre-change snapshot for 1,019,997 executions, with no observed mismatch. Fuzz input
was limited to 4 KiB; this is not proof of safety for unlimited resource usage.

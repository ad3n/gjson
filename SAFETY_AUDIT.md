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

# Syntax coverage audit

All feature categories documented in [SYNTAX.md](SYNTAX.md) are exercised by
[syntax_test.go](syntax_test.go), using the public `github.com/ad3n/gjson` import.
The package identifier remains `gjson`.

The suite contains **106 scenarios and 308 API/formatting checks**:

- 46 examples read directly from the document: 43 table rows and 3 paired
  path/output blocks. Fixtures are also read from the document.
- 34 operator, escaping, array, multipath, and literal boundary scenarios.
- 18 built-in modifier scenarios covering all 13 documented modifiers.
- 5 exact formatting checks for `@pretty` and its four documented options.
- 3 custom modifier argument/chaining scenarios.

Except for the exact formatting checks, each scenario runs through `Get`,
`GetBytes`, and `Result.Get`. The byte input is overwritten after `GetBytes` to
verify result ownership. Expected JSON is checked independently with
`encoding/json`; insignificant whitespace is normalized, while field order,
duplicate keys, scalar types, decoded strings, missing values, and explicit null
remain distinguishable.

| Documented feature | Coverage |
|---|---|
| Basic paths and array indices | Eight document rows; missing-index boundary |
| `*` and `?` wildcards | Document rows; zero-character, one-character, and no-match cases |
| Escaping | Document dot example; literal `*`, `?`, `|`, backslash, `#`, `@`, `!` keys |
| Array counts and projections | Document rows; empty arrays and missing/null child fields |
| First/all queries | Object and scalar document queries; no-match cases |
| `=`, `==`, `!=`, `<`, `<=`, `>`, `>=` | Explicit independent expected results |
| `%` and `!%` | Document object and scalar pattern queries |
| Nested queries | Document `nets` existence query |
| Legacy `#[...]` queries | First and all-result scenarios |
| `~true`, `~false`, `~null`, `~*`, negated existence | Five document rows with mixed values and absent fields |
| Dot versus pipe | All 13 document rows, including projection, indexing, counting, and missing results |
| Built-in modifiers | `reverse`, `ugly`, `pretty`, `this`, `valid`, `flatten`, `join`, `keys`, `values`, `tostr`, `fromstr`, `group`, `dig` |
| Modifier arguments | All pretty options; deep flatten; duplicate-preserving join; raw, object, array, and quoted custom arguments |
| Custom modifiers | Document uppercase/lowercase examples and chaining |
| Multipaths | Document named object output; array output, omitted missing values, default `_` name |
| Literals | Document object construction; string, number, booleans, null, object and array traversal |

The tests also check that every modifier listed in the document has an explicit
scenario and is registered. Section count guards prevent accidentally skipping
an existing category when parsing the document.

## Documentation corrections

- Examples returning complete friend objects now include `nets`, matching the
  input fixture. This also applies to the sorted pretty-print example.
- Scalar queries omit the field path on the **left** of the operator.
- The custom modifier chain is `children.@case:lower|@reverse`. With an unquoted
  argument, the dot in `lower.@reverse` belongs to the argument; it is not a
  chaining separator.
- `@group` groups parallel arrays from an object into an array of objects.

These corrections describe existing behavior: the same syntax suite passes
against the original baseline commit as well as the current implementation.
The parser implementation was not changed during this audit.

## Validation

- Go 1.26.8: all 308 syntax checks passed using the new public import path.
- Full test suite with shuffled order and coverage passed: **96.1% of statements**.
- `go vet ./...` and the full race suite passed on Go 1.26.8.
- Full test suite passed on Go 1.27.1.
- The same syntax suite passed against baseline commit
  `8d89927eff414537088a6092d53fecf6711c1e75`; the differential suite also passed.
- The dependency graph resolves this module as `github.com/ad3n/gjson` and does
  not depend on `github.com/tidwall/gjson`.
- `go fix -diff` has no pending changes. `git diff --check` passes.
- `betteralign -apply` was reviewed and its public `Result` reorder was reverted.
  The one remaining layout diagnostic is intentional to preserve public field
  order and offsets. No production parser code changed.

Statement coverage is distinct from feature coverage: **96.1%** does not mean all
possible input combinations or branches have been proved correct. Every feature
category in the current syntax document now has an explicit checked example.

```sh
GOTOOLCHAIN=go1.26.8 go test -run '^TestSyntax' -count=1 -v ./...
GOTOOLCHAIN=go1.26.8 go test -shuffle=on -coverprofile=coverage.out ./...
GOTOOLCHAIN=go1.26.8 go vet ./...
GOTOOLCHAIN=go1.26.8 go test -race ./...
GOTOOLCHAIN=go1.26.8 sh scripts/compare-baseline.sh -count=1
```

The comparison script runs the syntax suite against a temporary baseline copy
before comparing public declarations, `Result` layout, and result behavior.

## Module migration

Use `import "github.com/ad3n/gjson"` and `go get github.com/ad3n/gjson`.
The module declaration, consumer-facing documentation, and differential harness
use this path. Upstream commit/issue links, dependencies on `tidwall/match` and
`tidwall/pretty`, licensing, and historical benchmark metadata retain their
original identities. This audit does not change the git remote or publish the repository.

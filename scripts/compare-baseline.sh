#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
baseline=${BASELINE_REF:-8d89927eff414537088a6092d53fecf6711c1e75}
workspace=$(mktemp -d "${TMPDIR:-/tmp}/gjson-compatibility.XXXXXX")
trap 'status=$?; if [ "$status" -eq 0 ]; then rm -rf "$workspace"; else printf "Compatibility workspace retained: %s\n" "$workspace" >&2; fi' EXIT
trap 'exit 130' HUP INT TERM

mkdir "$workspace/baseline"
git -C "$root" show "$baseline:gjson.go" > "$workspace/baseline/gjson.go"
git -C "$root" show "$baseline:go.mod" | sed 's|^module .*|module baseline.example/gjson|' > "$workspace/baseline/go.mod"
git -C "$root" show "$baseline:go.sum" > "$workspace/baseline/go.sum"
cp "$root/testdata/compatibility_test.go" "$workspace/compatibility_test.go"
cp "$root/SYNTAX.md" "$workspace/baseline/SYNTAX.md"
sed 's|github.com/ad3n/gjson|baseline.example/gjson|g' "$root/syntax_test.go" > "$workspace/baseline/syntax_test.go"
cp "$workspace/baseline/gjson.go" "$workspace/baseline.txt"
cp "$root/gjson.go" "$workspace/current.txt"
cp "$root/go.sum" "$workspace/go.sum"
cat > "$workspace/go.mod" <<EOF
module compatibility

go 1.26.0

require (
 baseline.example/gjson v0.0.0
 github.com/ad3n/gjson v0.0.0
)

replace baseline.example/gjson => "$workspace/baseline"
replace github.com/ad3n/gjson => "$root"
EOF

(cd "$workspace/baseline" && GOTOOLCHAIN=${GOTOOLCHAIN:-go1.26.8} go test -mod=readonly -run '^TestSyntax' .)

cd "$workspace"
GOTOOLCHAIN=${GOTOOLCHAIN:-go1.26.8} go test -mod=mod "$@" .

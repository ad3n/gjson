#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
baseline=${BASELINE_REF:-8d89927eff414537088a6092d53fecf6711c1e75}
count=${BENCH_COUNT:-10}
duration=${BENCH_TIME:-200ms}
output=${1:-"$root/benchmarks"}
workspace=$(mktemp -d "${TMPDIR:-/tmp}/gjson-benchmark.XXXXXX")
trap 'rm -rf "$workspace"' EXIT HUP INT TERM
mkdir -p "$workspace/before" "$workspace/after" "$output"
output=$(CDPATH= cd -- "$output" && pwd)
export GOTOOLCHAIN=${GOTOOLCHAIN:-go1.26.8}

for file in gjson.go go.mod go.sum; do
    if [ -n "${BASELINE_DIR:-}" ]; then
        cp "$BASELINE_DIR/$file" "$workspace/before/$file"
    else
        git -C "$root" show "$baseline:$file" > "$workspace/before/$file"
    fi
    cp "$root/$file" "$workspace/after/$file"
done

for version in before after; do
    cp "$root/benchmark_test.go" "$workspace/$version/benchmark_test.go"
    (cd "$workspace/$version" && go test -c -o "$workspace/$version.test")
    : > "$output/$version.txt"
done

{
    go version
    printf 'baseline: %s\nsamples: %s\nbenchtime: %s\ncpu: 1\n' "$baseline" "$count" "$duration"
    printf 'baseline directory: %s\n' "${BASELINE_DIR:-git}"
    shasum -a 256 "$workspace/before/gjson.go"
    shasum -a 256 "$root/gjson.go" "$root/benchmark_test.go"
} > "$output/environment.txt"

i=0

while [ "$i" -lt "$count" ]; do
    order='before after'

    if [ "$((i % 2))" -eq 1 ]; then
        order='after before'
    fi

    for version in $order; do
        "$workspace/$version.test" -test.run '^$' -test.bench . -test.benchmem -test.benchtime "$duration" -test.cpu 1 >> "$output/$version.txt"
    done

    i=$((i + 1))
    printf 'Completed benchmark pair %s/%s\n' "$i" "$count"
done

package gjson

import (
	"encoding/json"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestJoinPoolRelease(t *testing.T) {
	keys := []Result{{Raw: "sensitive", Str: "sensitive", Indexes: []int{1}}}
	values := map[string]Result{"sensitive": keys[0]}
	buffer := &joinBuffer{keys: keys, values: values}
	releaseJoinBuffer(buffer)
	if len(values) != 0 || !reflect.DeepEqual(keys[0], Result{}) || len(buffer.keys) != 0 {
		t.Fatal("pool retained input references")
	}
}

func TestJoinPoolOwnership(t *testing.T) {
	for _, tc := range []struct{ doc, path, want string }{
		{`[]`, `@join`, `{}`},
		{`[{},1,null,[]]`, `@join`, `{}`},
		{`[{"a":1,"b":2},{"a":12345},{"a":3}]`, `@join`, `{"a":3,"b":2}`},
		{`[{"a":1},{"\u0061":2}]`, `@join`, `{"a":2}`},
		{`[{"\u0061":1},{"a":2}]`, `@join`, `{"\u0061":2}`},
		{`[{"a":1},{"a":2}]`, `@join:{"preserve":true}`, `{"a":1,"a":2}`},
		{`[{"v":{"nested":[1,2]}},{"s":"hello\nworld"}]`, `@join`, `{"v":{"nested":[1,2]},"s":"hello\nworld"}`},
	} {
		retained := Get(tc.doc, tc.path)
		data := []byte(tc.doc)
		retainedBytes := GetBytes(data, tc.path)
		clear(data)
		for range 50 {
			Get(`[{"replacement":"value"}]`, "@join")
		}
		runtime.GC()
		if retained.Raw != tc.want || retainedBytes.Raw != tc.want {
			t.Fatalf("%s: got %q / %q, want %q", tc.doc, retained.Raw, retainedBytes.Raw, tc.want)
		}
	}
}

func TestJoinPoolParallel(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				if got := Get(`[{"a":1,"b":2},{"a":3}]`, "@join").Raw; got != `{"a":3,"b":2}` {
					t.Errorf("concurrent join: %q", got)
				}
			}
		})
	}

	wg.Wait()
	large := `[{"a":"` + strings.Repeat("x", 20*1024) + `"}]`
	if got := Get(large, "@join").Raw; got != large[1:len(large)-1] {
		t.Fatal("large join output changed")
	}

	if got := Get(`[{"a":1}]`, "@join").Raw; got != `{"a":1}` {
		t.Fatal("reuse after large input failed")
	}
}

func TestEmptyModifierAllocations(t *testing.T) {
	for _, tc := range []struct{ doc, path, want string }{
		{`[]`, "@join", `{}`},
		{`[]`, "@flatten", `[]`},
		{`{}`, "@keys", `[]`},
		{`{}`, "@values", `[]`},
		{`[]`, "@keys", `[]`},
		{" [] ", "@values", "[] "},
	} {
		if got := Get(tc.doc, tc.path).Raw; got != tc.want {
			t.Fatalf("%q %s: %q", tc.doc, tc.path, got)
		}

		if n := testing.AllocsPerRun(100, func() { benchmarkResult = Get(tc.doc, tc.path) }); n != 0 {
			t.Fatalf("%s allocated %g times", tc.path, n)
		}
	}
}

func TestJoinPoolLimit(t *testing.T) {
	buffer := &joinBuffer{values: make(map[string]Result)}
	for i := range 257 {
		buffer.values[strconv.Itoa(i)] = Result{Raw: "input"}
	}

	releaseJoinBuffer(buffer)
	if buffer.values != nil || buffer.keys != nil {
		t.Fatal("oversized map retained")
	}

	buffer = &joinBuffer{values: make(map[string]Result), keys: make([]Result, 0, 257)}
	releaseJoinBuffer(buffer)
	if buffer.values != nil || buffer.keys != nil {
		t.Fatal("oversized slice retained")
	}
}

func TestBufferedUnescapeBoundaries(t *testing.T) {
	for _, size := range []int{31, 32, 33, 1023, 1024, 1025} {
		for _, prefix := range []string{`\u0041`, `\ud83d\ude00`, `\ud800`, `\udc00`} {
			input := `"` + prefix + strings.Repeat("a", size-len(prefix)) + `"`
			var want string
			if err := json.Unmarshal([]byte(input), &want); err != nil {
				t.Fatal(err)
			}

			retained := Parse(input).Str
			for range 20 {
				Parse(`"\u0042` + strings.Repeat("x", size) + `"`)
			}

			if retained != want {
				t.Fatalf("size %d: got %q, want %q", size, retained, want)
			}
		}
	}
}

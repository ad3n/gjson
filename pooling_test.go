package gjson

import (
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestReverseBufferRelease(t *testing.T) {
	values := make([]Result, 8)
	for i := range values {
		values[i] = Result{Raw: "input", Str: "value", Indexes: []int{1}}
	}
	buffer := &reverseBuffer{values: values}
	releaseReverseBuffer(buffer)
	for _, value := range values {
		if !reflect.DeepEqual(value, Result{}) {
			t.Fatal("pool retains input references")
		}
	}

	oversized := &reverseBuffer{values: make([]Result, 1025)}
	releaseReverseBuffer(oversized)
	if oversized.values != nil {
		t.Fatal("oversized backing array retained")
	}
}

func TestReversePoolOwnership(t *testing.T) {
	for _, tc := range []struct{ doc, want string }{
		{`[1,2,3]`, `[3,2,1]`},
		{`{"a":1,"a":2,"b":3}`, `{"b":3,"a":2,"a":1}`},
		{`[]`, `[]`}, {`{}`, `{}`},
	} {
		retained := Get(tc.doc, "@reverse")
		for range 50 {
			Get(`["replacement",{"nested":true}]`, "@reverse")
			Get(`{"other":"value"}`, "@reverse")
		}

		runtime.GC()
		if retained.Raw != tc.want || Get(tc.doc, "@reverse").Raw != tc.want {
			t.Fatal("pooled storage changed returned result or failed after GC")
		}
	}
}

func TestReversePoolParallel(t *testing.T) {
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				if Get(`[1,2,3]`, "@reverse").Raw != `[3,2,1]` || Get(`{"a":1,"b":2}`, "@reverse").Raw != `{"b":2,"a":1}` {
					t.Error("parallel reverse changed output")
					return
				}
			}
		})
	}

	wg.Wait()
	large := "[" + strings.Repeat("1,", 2048) + "2]"
	if got := Get(large, "@reverse").Raw; got != "[2,"+strings.Repeat("1,", 2047)+"1]" {
		t.Fatal("oversized reverse failed")
	}
	if Get(`[1,2]`, "@reverse").Raw != `[2,1]` {
		t.Fatal("reuse after oversized input failed")
	}
}

package gjson

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestPathInvalidMetadata(t *testing.T) {
	for _, tc := range []struct {
		doc    string
		result Result
	}{
		{"{}", Result{Index: -1}},
		{"{}", Result{Index: math.MaxInt, Raw: "x"}},
		{"{}", Result{Index: 3}},
		{":1", Result{Type: Number, Raw: "1", Index: 1}},
		{`"v":1`, Result{Type: Number, Raw: "1", Index: 4}},
	} {
		if got := tc.result.Path(tc.doc); got != "" {
			t.Fatalf("Path(%q, %#v) = %q", tc.doc, tc.result, got)
		}
	}
}

func TestConcurrentModifiers(t *testing.T) {
	const name = "safety_concurrent"
	identity := func(doc, arg string) string { return doc }
	AddModifier(name, identity)
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for range 200 {
				if worker%2 == 0 {
					AddModifier(name, identity)
					continue
				}

				if !ModifierExists(name, nil) || Get(`{"v":1}`, "@"+name+"|v").Int() != 1 || Get(`{"v":1}`, "v.@"+name).Int() != 1 {
					t.Error("concurrent modifier lookup failed")
					return
				}
			}
		})
	}

	wg.Wait()
	for worker := range 8 {
		wg.Go(func() { AddModifier("safety_writer_"+strconv.Itoa(worker), identity) })
	}

	wg.Wait()
	for worker := range 8 {
		if !ModifierExists("safety_writer_"+strconv.Itoa(worker), nil) {
			t.Fatal("concurrent registration lost a modifier")
		}
	}
}

func TestModifierReentrancy(t *testing.T) {
	AddModifier("safety_reentrant", func(doc, arg string) string {
		AddModifier("safety_inner", func(doc, arg string) string { return doc })
		return Get(doc, "@safety_inner").Raw
	})
	if got := Get(`{"v":1}`, "@safety_reentrant|v").Int(); got != 1 {
		t.Fatalf("reentrant modifier returned %d", got)
	}
}

func TestNilModifier(t *testing.T) {
	AddModifier("safety_nil", nil)
	if !ModifierExists("safety_nil", nil) {
		t.Fatal("registered name disappeared")
	}

	if Get(`{"v":1}`, "@safety_nil").Exists() {
		t.Fatal("nil modifier should not execute")
	}
}

func FuzzPublicAPISafety(f *testing.F) {
	for _, doc := range []string{"", "{}", "[]", ":1", `"v":1`, `{"v":[1,null,"x"]}`, `{"v":"\uD800"}`, strings.Repeat("[", 64) + "1" + strings.Repeat("]", 64)} {
		f.Add(doc, "v", 0)
	}

	f.Fuzz(func(t *testing.T, doc, path string, index int) {
		if len(doc) > 4096 || len(path) > 256 {
			t.Skip()
		}

		Valid(doc)
		ValidBytes([]byte(doc))
		Escape(path)
		AppendJSONString(nil, doc)
		for _, r := range []Result{Parse(doc), Get(doc, path), GetBytes([]byte(doc), path)} {
			r.Array()
			r.Map()
			r.Value()
			r.Path(doc)
			r.Paths(doc)
			r.Get(path)
			r.ForEach(func(_, value Result) bool { return true })
		}

		(Result{Raw: "x", Index: index}).Path(doc)
		ForEachLine(doc, func(Result) bool { return true })
	})
}

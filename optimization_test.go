package gjson

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestBoolCompatibility(t *testing.T) {
	check := func(s string) {
		t.Helper()
		want, _ := strconv.ParseBool(strings.ToLower(s))
		got := (Result{Type: String, Str: s}).Bool()

		if got != want {
			t.Fatalf("Bool(%q) = %v, want %v", s, got, want)
		}
	}

	for _, s := range []string{"", "0", "1", "true", "false", "yes", " true", "true ", "\xff", "tr\x00e"} {
		check(s)
	}

	for _, s := range []string{"t", "f", "true", "false"} {
		for mask := range 1 << len(s) {
			value := []byte(s)

			for i := range value {
				if mask&(1<<i) != 0 {
					value[i] -= 'a' - 'A'
				}
			}

			check(string(value))
		}
	}

	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !strings.ContainsRune("true", unicode.ToLower(r)) {
			continue
		}

		for i := range len("true") {
			check("true"[:i] + string(r) + "true"[i+1:])
		}
	}
}

func TestProjectionMetadata(t *testing.T) {
	doc := `{"items":[{"id":1},{"skip":0},{"id":null},{"id":"x"}]}`
	r := Get(doc, "items.#.id")

	if r.Raw != `[1,null,"x"]` || !reflect.DeepEqual(r.Indexes, []int{16, 36, 48}) {
		t.Fatalf("unexpected projection: %#v", r)
	}

	for _, doc := range []string{"[]", `[{},{}]`} {
		r := Get(doc, "#.id")

		if r.Raw != "[]" || r.Indexes == nil || len(r.Indexes) != 0 {
			t.Fatalf("empty projection changed: %#v", r)
		}
	}
}

func TestNumericQueryCompatibility(t *testing.T) {
	doc := `[{"id":1},{"id":"2"},{"id":2},{"id":null},{"id":3}]`

	for _, tc := range []struct{ path, want string }{
		{`#(id>=2)#.id`, `["2",2,3]`},
		{`#(id>invalid)#.id`, `[1,2,3]`},
		{`#(id>1e999)#.id`, `["2"]`},
		{`#(id!=NaN)#.id`, `[1,"2",2,3]`},
		{`#(id=~true)#.id`, `[1,2,3]`},
		{`#(id>=2).id`, `"2"`},
	} {
		if got := Get(doc, tc.path); got.Raw != tc.want {
			t.Errorf("%s: got %s, want %s", tc.path, got.Raw, tc.want)
		}
	}
}

func TestGetBytesOwnershipAfterDecode(t *testing.T) {
	for _, doc := range []string{
		`{"v":"plain"}`,
		`{"v":"hello\nworld"}`,
		`{"v":"\ud83d\ude00"}`,
		`{"v":"` + strings.Repeat("abc", 100) + `\n"}`,
	} {
		data := []byte(doc)
		want := Get(doc, "v")
		got := GetBytes(data, "v")
		clear(data)

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("input mutation changed result: got %#v, want %#v", got, want)
		}
	}
}

func TestLongEscapeOutputs(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{strings.Repeat(`\u0041`, 128), strings.Repeat("A", 128)},
		{`\z` + strings.Repeat("a", 128), ""},
		{`a\z` + strings.Repeat("b", 128), "a"},
		{strings.Repeat("a", 128) + `\ud800`, strings.Repeat("a", 128) + "�"},
	} {
		if got := unescape(tc.input); got != tc.want {
			t.Fatalf("unescape(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestZeroAllocationPaths(t *testing.T) {
	checks := map[string]func(){
		"single escape": func() { benchmarkResult = Get(`{"v":"\n"}`, "v") },
		"unicode ascii": func() { benchmarkResult = Get(`{"v":"\u0041"}`, "v") },
		"lookup":        func() { benchmarkResult = Get(`{"v":"plain"}`, "v") },
		"bool mixed":    func() { benchmarkBool = (Result{Type: String, Str: "TrUe"}).Bool() },
		"bool invalid":  func() { benchmarkBool = (Result{Type: String, Str: "invalid"}).Bool() },
		"iteration": func() {
			Parse(`[1,2,3]`).ForEach(func(_, value Result) bool {
				benchmarkResult = value
				return true
			})
		},
	}

	for name, run := range checks {
		t.Run(name, func(t *testing.T) {
			if n := testing.AllocsPerRun(100, run); n != 0 {
				t.Fatalf("got %g allocations, want zero", n)
			}
		})
	}
}

func FuzzBoolCompatibility(f *testing.F) {
	for _, s := range []string{"", "TrUe", "FALSE", "1", "0", "invalid", "\xff"} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		want, _ := strconv.ParseBool(strings.ToLower(s))

		if got := (Result{Type: String, Str: s}).Bool(); got != want {
			t.Fatalf("Bool(%q) = %v, want %v", s, got, want)
		}
	})
}

func FuzzStringDecode(f *testing.F) {
	for _, s := range []string{"", "plain", "a\nb", "😀", "\x00\xff", strings.Repeat("long", 50)} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		encoded, err := json.Marshal(s)

		if err != nil {
			t.Fatal(err)
		}

		var want string

		if err := json.Unmarshal(encoded, &want); err != nil {
			t.Fatal(err)
		}

		got := ParseBytes(encoded)
		clear(encoded)

		if got.Str != want {
			t.Fatalf("decoded %q, want %q", got.Str, want)
		}
	})
}

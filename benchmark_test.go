package gjson

import (
	"strconv"
	"strings"
	"testing"
)

var benchmarkResult Result
var benchmarkResults []Result
var benchmarkBool bool
var benchmarkBytes []byte

func BenchmarkLookup(b *testing.B) {
	doc := `{"name":"Anderson","escaped":"hello\nworld","unicode":"\u0041","single":"\n","long":"` +
		strings.Repeat("hello", 20) + `\nworld","age":37,"items":[{"id":1},{"id":2},{"id":3}]}`
	data := []byte(doc)

	for _, path := range []string{"name", "escaped", "unicode", "single", "long", "age", "missing"} {
		b.Run("Get/"+path, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				benchmarkResult = Get(doc, path)
			}
		})
		b.Run("GetBytes/"+path, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				benchmarkResult = GetBytes(data, path)
			}
		})
	}
}

func BenchmarkBool(b *testing.B) {
	for _, value := range []string{"true", "TrUe", "FALSE", "invalid"} {
		b.Run(value, func(b *testing.B) {
			r := Result{Type: String, Str: value}
			b.ReportAllocs()

			for b.Loop() {
				benchmarkBool = r.Bool()
			}
		})
	}
}

func BenchmarkDecodeDensity(b *testing.B) {
	for _, value := range []string{strings.Repeat(`\u0041`, 128), `\z` + strings.Repeat("a", 128)} {
		name := "unicode"

		if strings.HasPrefix(value, `\z`) {
			name = "invalid"
		}

		b.Run(name, func(b *testing.B) {
			doc := `{"v":"` + value + `"}`
			b.ReportAllocs()

			for b.Loop() {
				benchmarkResult = Get(doc, "v")
			}
		})
	}
}

func BenchmarkProjection(b *testing.B) {
	for _, size := range []int{0, 3, 64, 1000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			doc := "[" + strings.TrimSuffix(strings.Repeat(`{"id":123},`, size), ",") + "]"
			b.ReportAllocs()

			for b.Loop() {
				benchmarkResult = Get(doc, "#.id")
			}
		})
	}
}

func BenchmarkQuery(b *testing.B) {
	doc := "[" + strings.Repeat(`{"id":1},`, 999) + `{"id":1000}]`

	for _, path := range []string{`#(id>=1000).id`, `#(id>1000).id`, `#(id>=1000)#.id`, `#(id>invalid).id`, `#(id<invalid).id`} {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				benchmarkResult = Get(doc, path)
			}
		})
	}
}

func BenchmarkIteration(b *testing.B) {
	r := Parse(`[{"id":1},{"id":2},{"id":3}]`)
	b.Run("Array", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			benchmarkResults = r.Array()
		}
	})
	b.Run("ForEach", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			r.ForEach(func(_, value Result) bool {
				benchmarkResult = value
				return true
			})
		}
	})
	b.Run("Values", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			for value := range r.Values() {
				benchmarkResult = value
			}
		}
	})
}

func BenchmarkAppendReuse(b *testing.B) {
	buf := make([]byte, 0, 128)
	b.ReportAllocs()

	for b.Loop() {
		benchmarkBytes = AppendJSONString(buf[:0], "hello\nworld")
	}
}

func BenchmarkModifierLookup(b *testing.B) {
	doc := `{"v":1}`
	for _, path := range []string{"@this", "@this|v", "v.@this", "@missing"} {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkResult = Get(doc, path)
			}
		})
	}
}

func BenchmarkReverse(b *testing.B) {
	for _, size := range []int{0, 3, 64, 1000, 2048} {
		array := "[" + strings.TrimSuffix(strings.Repeat(`{"v":123},`, size), ",") + "]"
		object := "{" + strings.TrimSuffix(strings.Repeat(`"v":123,`, size), ",") + "}"
		for _, tc := range []struct{ name, doc string }{{"array", array}, {"object", object}} {
			b.Run(tc.name+"/"+strconv.Itoa(size), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					benchmarkResult = Get(tc.doc, "@reverse")
				}
			})
		}
	}
}

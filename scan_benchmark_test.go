package gjson

import (
	"strconv"
	"strings"
	"testing"
)

func BenchmarkASMHotspot(b *testing.B) {
	for _, size := range []int{16, 256, 4096, 65536} {
		plain := strings.Repeat("a", size)
		doc := `{"payload":"` + plain + `","target":1}`
		nested := `{"skip":{"payload":"` + plain + `"},"target":1}`
		escaped := `{"payload":"` + strings.Repeat(`aaaaaaa\n`, size/8) + `","target":1}`
		for _, name := range []string{"string", "tail", "missing", "nested", "valid", "escaped", "quotes", "mixed_quotes", "append"} {
			b.Run(name+"/"+strconv.Itoa(size), func(b *testing.B) {
				input := doc
				path := "payload"
				switch name {
				case "tail":
					path = "target"
				case "missing":
					path = "missing"
				case "nested":
					input, path = nested, "target"
				case "escaped":
					input = escaped
				case "quotes":
					input = `{"payload":"` + strings.Repeat(`\"`, size/2) + `","target":1}`
				case "mixed_quotes":
					input = `{"payload":"\n` + strings.Repeat(`\"`, size/2) + `","target":1}`
				}

				bytes := len(input)
				if name == "string" || name == "escaped" || name == "quotes" || name == "mixed_quotes" {
					bytes -= len(`,"target":1}`)
				}

				if name == "append" {
					bytes = len(plain)
				}

				b.SetBytes(int64(bytes))
				b.ReportAllocs()
				buf := make([]byte, 0, len(plain)+2)
				switch name {
				case "valid":
					if !Valid(input) {
						b.Fatal("invalid fixture")
					}

					for b.Loop() {
						benchmarkBool = Valid(input)
					}
				case "append":
					for b.Loop() {
						benchmarkBytes = AppendJSONString(buf[:0], plain)
					}
				default:
					result := Get(input, path)
					if (name == "missing") == result.Exists() {
						b.Fatal("unexpected lookup result")
					}

					for b.Loop() {
						benchmarkResult = Get(input, path)
					}
				}
			})
		}
	}
}

package gjson

import (
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestIndexStringSpecial(t *testing.T) {
	for offset := 0; offset < 32; offset++ {
		for size := 0; size <= 97; size++ {
			buf := []byte(strings.Repeat("a", offset+size+32))
			for pos := 0; pos <= size; pos++ {
				for _, special := range []byte{'"', '\\'} {
					buf[offset+pos] = special
					s := string(buf)[offset : offset+size]
					if got := indexStringSpecial(s); got != pos {
						t.Fatalf("offset=%d size=%d pos=%d special=%d got=%d", offset, size, pos, special, got)
					}

					buf[offset+pos] = 'a'
				}
			}
		}
	}
}

func TestParseStringSIMD(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n < 10000; n++ {
		buf := make([]byte, rng.Intn(2048)+1)
		_, _ = rng.Read(buf)
		i := 1 + rng.Intn(len(buf))
		assertParseString(t, string(buf), i)
	}

	for size := 0; size < 128; size++ {
		for _, suffix := range []string{`"`, `\"`, `\\"`, `\\\"`, `\`, ""} {
			assertParseString(t, `"`+strings.Repeat("a", size)+suffix, 1)
		}
	}
}

func FuzzParseStringSIMD(f *testing.F) {
	for _, s := range []string{"", `"`, strings.Repeat("a", 64), strings.Repeat("a", 32) + `\\"`} {
		f.Add(s, uint16(0))
	}

	f.Fuzz(func(t *testing.T, s string, offset uint16) {
		input := `"` + s
		i := 1 + int(offset)%len(input)
		assertParseString(t, input, i)
	})
}

func assertParseString(t testing.TB, s string, i int) {
	t.Helper()
	a, b, c, d := parseString(s, i)
	w, x, y, z := parseStringReference(s, i)
	if a != w || b != x || c != y || d != z {
		t.Fatalf("input=%q start=%d got=(%d,%q,%t,%t) want=(%d,%q,%t,%t)", s, i, a, b, c, d, w, x, y, z)
	}
}

func parseStringReference(json string, i int) (int, string, bool, bool) {
	var s = i
	for ; i < len(json); i++ {
		if json[i] > '\\' {
			continue
		}

		if json[i] == '"' {
			return i + 1, json[s-1 : i+1], false, true
		}

		if json[i] == '\\' {
			i++
			for ; i < len(json); i++ {
				if json[i] > '\\' {
					continue
				}

				if json[i] == '"' {
					if json[i-1] == '\\' {
						n := 0
						for j := i - 2; j > 0; j-- {
							if json[j] != '\\' {
								break
							}

							n++
						}

						if n%2 == 0 {
							continue
						}
					}

					return i + 1, json[s-1 : i+1], true, true
				}
			}

			break
		}
	}

	return i, json[s-1:], false, false
}

func TestStringScanBlockBoundaries(t *testing.T) {
	for _, size := range []int{stringScanBlockSize - 1, stringScanBlockSize, stringScanBlockSize + 1, 2*stringScanBlockSize + 17} {
		for _, pos := range []int{0, 15, 16, size / 2, size - 1, size} {
			for _, special := range []byte{'"', '\\'} {
				buf := []byte(strings.Repeat("a", size+1))
				buf[pos] = special
				input := string(buf[:size])
				if got := indexStringSpecial(input); got != pos {
					t.Fatalf("size=%d pos=%d got=%d", size, pos, got)
				}

				assertParseString(t, `"`+input, 1)
			}
		}
	}
}

func TestStringScanConcurrent(t *testing.T) {
	input := `"` + strings.Repeat("a", stringScanBlockSize*2) + `\nend"`
	want := len(input)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 128 {
				n, raw, escaped, ok := parseString(input, 1)
				if n != want || raw != input || !escaped || !ok {
					t.Error("concurrent parse mismatch")
					return
				}
			}
		})
	}

	wg.Go(func() {
		for range 8 {
			runtime.GC()
		}
	})
	wg.Wait()
}

func TestStringScanAllocations(t *testing.T) {
	input := `"` + strings.Repeat("a", stringScanBlockSize*2) + `"`
	if allocations := testing.AllocsPerRun(100, func() {
		n, raw, escaped, ok := parseString(input, 1)
		if n != len(input) || raw != input || escaped || !ok {
			t.Fatal("parse mismatch")
		}
	}); allocations != 0 {
		t.Fatalf("allocations=%v", allocations)
	}
}

func FuzzIndexStringSpecial(f *testing.F) {
	for _, s := range []string{"", `"`, `\`, strings.Repeat("a", stringScanBlockSize+1), strings.Repeat("a", 127) + `\"`} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		want := strings.IndexAny(s, "\"\\")
		if want < 0 {
			want = len(s)
		}

		if got := indexStringSpecial(s); got != want {
			t.Fatalf("got=%d want=%d input=%q", got, want, s)
		}
	})
}

func TestParseStringShortEscape(t *testing.T) {
	for offset := 0; offset < 16; offset++ {
		for c := 0; c < 256; c++ {
			input := strings.Repeat("a", offset) + "\"\\" + string(byte(c)) + "\"suffix"
			assertParseString(t, input, offset+1)
		}
	}
}

func TestParseStringEscapedBlocks(t *testing.T) {
	for _, size := range []int{63, 64, 65, stringScanBlockSize - 1, stringScanBlockSize, stringScanBlockSize + 1} {
		for _, content := range []string{strings.Repeat("a", size), strings.Repeat(`\"`, size/2), strings.Repeat(`a\"`, size/3)} {
			for _, suffix := range []string{`"`, `\"`, `\\"`, ""} {
				assertParseString(t, `"\n`+content+suffix, 1)
			}
		}
	}
}

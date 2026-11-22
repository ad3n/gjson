package gjson

import (
	"strconv"
	"strings"
	"testing"
)

func TestParseUintInvalidTwentyBytes(t *testing.T) {
	for _, digit := range []string{"1", "9"} {
		for i := range 20 {
			for _, invalid := range []byte{'/', ':', '.', 'e', '-', 0, 255} {
				input := []byte(strings.Repeat(digit, 20))
				input[i] = invalid
				if n, ok := parseUint(string(input)); ok || n != 0 {
					t.Fatalf("parseUint(%q) = %d, %v", input, n, ok)
				}
			}
		}
	}
}

func FuzzParseUint(f *testing.F) {
	for _, input := range []string{"", "0", "00000000000000000001", "0000000000000000000001", "0000000000000000000000", "00018446744073709551616", "00000000000000000000x", "100000000000000000000", "18446744073709551615", "18446744073709551616", "99999999999999999999", "2026-09-17T18:04:00Z"} {
		f.Add(input)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var want uint64
		valid := len(input) > 0 && len(strings.TrimLeft(input, "0")) <= 20
		for i := range len(input) {
			if input[i] < '0' || input[i] > '9' {
				valid = false
				break
			}
		}

		if valid {
			want, _ = strconv.ParseUint(input, 10, 64)
		}

		if got, ok := parseUint(input); got != want || ok != valid {
			t.Fatalf("parseUint(%q) = %d, %v; want %d, %v", input, got, ok, want, valid)
		}
	})
}

package gjson

import (
	"strconv"
	"strings"
	"testing"
)

func BenchmarkJoin(b *testing.B) {
	for _, size := range []int{0, 3, 64, 512} {
		var doc strings.Builder
		doc.WriteByte('[')
		for i := range size {
			if i > 0 {
				doc.WriteByte(',')
			}
			doc.WriteString(`{"k` + strconv.Itoa(i) + `":123}`)
		}
		doc.WriteByte(']')
		input := doc.String()
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkResult = Get(input, "@join")
			}
		})
	}
}

func BenchmarkEmptyModifiers(b *testing.B) {
	for _, path := range []string{"@join", "@flatten", "@keys", "@values"} {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkResult = Get(`{}`, path)
			}
		})
	}
}

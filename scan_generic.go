//go:build (!amd64 && !arm64) || purego

package gjson

const stringScanSIMD = false

func indexStringSpecialBlock(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			return i
		}
	}

	return len(s)
}

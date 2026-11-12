//go:build (amd64 || arm64) && !purego

package gjson

const stringScanSIMD = true

//go:noescape
func indexStringSpecialBlock(s string) int

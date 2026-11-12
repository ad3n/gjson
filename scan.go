package gjson

const stringScanBlockSize = 64 * 1024

func indexStringSpecial(s string) int {
	offset := 0
	for len(s) > stringScanBlockSize {
		i := indexStringSpecialBlock(s[:stringScanBlockSize])
		if i < stringScanBlockSize {
			return offset + i
		}

		offset += stringScanBlockSize
		s = s[stringScanBlockSize:]
	}

	return offset + indexStringSpecialBlock(s)
}

package awssigner

import (
	"strings"
)

func StripExcessSpaces(str string) string {
	var j, k, l, m, spaces int
	// Trim trailing spaces
	for j = len(str) - 1; j >= 0 && str[j] == ' '; j-- {
	}

	// Trim leading spaces
	for k = 0; k < j && str[k] == ' '; k++ {
	}
	str = str[k : j+1]

	// Strip multiple spaces.
	j = strings.Index(str, doubleSpace)
	if j < 0 {
		return str
	}

	buf := []byte(str)
	for k, m, l = j, j, len(buf); k < l; k++ {
		if buf[k] == ' ' {
			if spaces == 0 {
				// First space.
				buf[m] = buf[k]
				m++
			}
			spaces++
		} else {
			// End of multiple spaces.
			spaces = 0
			buf[m] = buf[k]
			m++
		}
	}

	return string(buf[:m])
}

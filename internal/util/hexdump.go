package util

import (
	"fmt"
	"unicode"
)

func hexLine(data []byte, length int) string {
	hex := ""
	ascii := ""
	for i := 0; i < length; i++ {
		if i < len(data) {
			hex += fmt.Sprintf("%02x  ", data[i])
			if unicode.IsPrint(rune(data[i])) {
				ascii += fmt.Sprintf("%c", data[i])
			} else {
				ascii += "."
			}
		} else {
			hex += "    "
			ascii += " "
		}
	}
	return hex + "| " + ascii
}

func HexDump(data []byte, start, len int) string {

	res := ""
	for len > 16 {
		res += fmt.Sprintf("%08x: %s\n", start, hexLine(data[start:], 16))
		start += 16
		len -= 16
	}
	if len > 0 {
		res += fmt.Sprintf("%08x: %s\n", start, hexLine(data[start:], len))
	}
	return res
}

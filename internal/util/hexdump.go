/*
 * This file is part of then Oberon Disk Image Tool ("odit")
 * Copyright (C) 2025 Andreas Signer <asigner@gmail.com>
 *
 * odit is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * odit is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with Oberon Disk Image Tool.  If not, see <https://www.gnu.org/licenses/>.
 */

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

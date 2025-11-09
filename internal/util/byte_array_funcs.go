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
	"bytes"
)

func WriteLEUint32(b []byte, offset int, value uint32) {
	b[offset] = byte(value & 0xFF)
	b[offset+1] = byte((value >> 8) & 0xFF)
	b[offset+2] = byte((value >> 16) & 0xFF)
	b[offset+3] = byte((value >> 24) & 0xFF)
}

func ReadLEUint32(b []byte, offset int) uint32 {
	return uint32(b[offset]) | uint32(b[offset+1])<<8 | uint32(b[offset+2])<<16 | uint32(b[offset+3])<<24
}

func WriteLEUint16(b []byte, offset int, value uint16) {
	b[offset] = byte(value & 0xFF)
	b[offset+1] = byte((value >> 8) & 0xFF)
}

func ReadLEUint16(b []byte, offset int) uint16 {
	return uint16(b[offset]) | uint16(b[offset+1])<<8
}

func StringFromBytes(b []byte) string {
	return string(bytes.TrimRight(b, "\x00"))
}

func WriteFixedLengthString(b []byte, offset int, length int, s string) {
	copy(b[offset:], s)
	for i := len(s); i < length; i++ {
		b[offset+i] = 0
	}
}

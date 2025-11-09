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

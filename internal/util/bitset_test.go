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
 * along with odit.  If not, see <https://www.gnu.org/licenses/>.
 */

package util

import (
	"testing"
)

func TestBitSet(t *testing.T) {
	bitset := NewBitSet(129)

	expected := []uint64{0, 0, 0}
	for i, v := range expected {
		if bitset[i] != v {
			t.Errorf("Expected bitset[%d] to be %d, got %d", i, v, bitset[i])
		}
	}

	bitset.Set(5)
	expected = []uint64{
		1 << 5, 0, 0,
	}
	for i, v := range expected {
		if bitset[i] != v {
			t.Errorf("Expected bitset[%d] to be %d, got %d", i, v, bitset[i])
		}
	}

	if !bitset.Test(5) {
		t.Errorf("Expected bit 5 to be set")
	}

	bitset.Clear(5)
	expected = []uint64{
		0, 0, 0,
	}
	for i, v := range expected {
		if bitset[i] != v {
			t.Errorf("Expected bitset[%d] to be %d, got %d", i, v, bitset[i])
		}
	}
	if bitset.Test(5) {
		t.Errorf("Expected bit 5 to be cleared")
	}
}

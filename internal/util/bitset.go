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

type BitSet []uint64

func NewBitSet(size uint32) BitSet {
	return make(BitSet, (size+63)/64)
}

func (b BitSet) Set(bit uint32) {
	b[bit/64] |= 1 << (bit % 64)
}

func (b BitSet) Clear(bit uint32) {
	b[bit/64] &^= 1 << (bit % 64)
}

func (b BitSet) Test(bit uint32) bool {
	return b[bit/64]&(1<<(bit%64)) != 0
}

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

package filesystem

import (
	"fmt"

	"github.com/asig/odit/internal/disk"
	"github.com/asig/odit/internal/util"
)

const (
	indexSize = disk.SectorSize / 4
)

type indexSector disk.Sector

/*
	IndexSector* =
		RECORD (Disk.Sector)
			x*: ARRAY IndexSize OF DiskAdr
		END ;
*/

func (i *indexSector) entries() []uint32 {
	addrs := make([]uint32, 0, indexSize)
	for j := 0; j < indexSize; j++ {
		adr := util.ReadLEUint32(i[:], j*4)
		if adr != 0 {
			addrs = append(addrs, adr)
		}
	}
	return addrs
}

func (i *indexSector) setEntry(index uint32, addr uint32) {
	if index >= indexSize {
		panic(fmt.Sprintf("index out of range: %d >= %d", index, indexSize))
	}
	util.WriteLEUint32(i[:], int(index*4), addr)
}

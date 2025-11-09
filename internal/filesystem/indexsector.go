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

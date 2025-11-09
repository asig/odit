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

package filesystem

import (
	"fmt"

	"github.com/asig/odit/internal/disk"
	"github.com/asig/odit/internal/util"
	"github.com/rs/zerolog/log"
)

const (
	dirPgSize    = 50
	dirEntrySize = fnLength + 8
	n            = dirPgSize / 2

	dirMark = 0x9B1EA38D
)

type dirEntry struct {
	name string
	adr  uint32
	p    *dirPage
}

type dirPage struct {
	parent  *dirPage
	addr    uint32
	m       uint16
	p0      *dirPage
	entries []dirEntry
}

/*
	DirEntry* =  (*B-tree node*)
		RECORD
			name*: FileName;	// Offset: 0
			adr*:  DiskAdr; 	// Offset: 32	(*sec no of file header*)
			p*:    DiskAdr  	// Offset: 36	(*sec no of descendant in directory*)
		END ;

	DirPage*  =
		RECORD (Disk.Sector)
			mark*:  LONGINT;	// Offset: 0
			m*:     INTEGER;	// Offset: 4
			p0*:    DiskAdr;    // Offset: 8     (*sec no of left descendant in directory*)
			fill:   ARRAY FillerSize OF CHAR; // Offset: 12
			e*:  ARRAY DirPgSize OF DirEntry	// Offset: 48
		END ;
*/

func loadDirFromDisk(d *disk.Disk, addr uint32, seen map[uint32]struct{}, parent uint32) (*dirPage, error) {
	sec := d.MustGetSector(addr)
	mark := util.ReadLEUint32(sec[:], 0)
	if mark != dirMark {
		return nil, fmt.Errorf("invalid dir page mark: got 0x%08X, want 0x%08X", mark, dirMark)
	}

	if _, ok := seen[addr]; ok {
		return nil, fmt.Errorf("detected cycle in directory pages at address %d, coming from %d", addr, parent)
	}
	seen[addr] = struct{}{}

	dir := &dirPage{
		addr:    addr,
		entries: make([]dirEntry, 0, dirPgSize),
	}

	pageAddrs := []uint32{}
	pageAddrs = append(pageAddrs, util.ReadLEUint32(sec[:], 8)) // p0
	for i := 0; i < int(util.ReadLEUint16(sec[:], 4)); i++ {
		offset := 48 + i*dirEntrySize
		pAddr := util.ReadLEUint32(sec[:], offset+fnLength+4)
		pageAddrs = append(pageAddrs, pAddr)
	}
	log.Debug().Msgf("Dir Page %04d: Child pages: %v", dir.addr, pageAddrs)

	p0Addr := util.ReadLEUint32(sec[:], 8)
	if p0Addr != 0 {
		var err error
		dir.p0, err = loadDirFromDisk(d, p0Addr, seen, addr)
		if err != nil {
			return nil, err
		}
	}

	m := util.ReadLEUint16(sec[:], 4)
	log.Debug().Msgf("Dir Page %04d: %02d entries", dir.addr, m)
	for i := 0; i < int(m); i++ {
		offset := 48 + i*dirEntrySize
		name := util.StringFromBytes(sec[offset : offset+fnLength])
		adr := util.ReadLEUint32(sec[:], offset+fnLength)
		pAddr := util.ReadLEUint32(sec[:], offset+fnLength+4)

		log.Debug().Msgf("Dir Page %04d: Entry %02d of %02d: %q", dir.addr, i, m, name)

		var p *dirPage
		if pAddr != 0 {
			var err error
			p, err = loadDirFromDisk(d, pAddr, seen, addr)
			if err != nil {
				return nil, err
			}
		}

		dir.entries = append(dir.entries, dirEntry{
			name: name,
			adr:  adr,
			p:    p,
		})
	}
	return dir, nil
}

func (dp *dirPage) asSector() disk.Sector {
	var sec disk.Sector
	util.WriteLEUint32(sec[:], 0, dirMark)
	util.WriteLEUint16(sec[:], 4, uint16(len(dp.entries)))
	util.WriteLEUint32(sec[:], 8, dp.p0.getAddr())

	for i, e := range dp.entries {
		offset := 48 + i*dirEntrySize
		util.WriteFixedLengthString(sec[:], offset, fnLength, e.name)
		util.WriteLEUint32(sec[:], offset+fnLength, e.adr)
		util.WriteLEUint32(sec[:], offset+fnLength+4, e.p.getAddr())
	}

	return sec
}

func (dp *dirPage) writeToDisk(d *disk.Disk) error {
	if dp == nil {
		return nil
	}

	sec := dp.asSector()
	d.MustPutSector(dp.addr, sec)
	if err := dp.p0.writeToDisk(d); err != nil {
		return err
	}
	for _, e := range dp.entries {
		if err := e.p.writeToDisk(d); err != nil {
			return err
		}
	}
	return nil
}

func (dp *dirPage) collectDirPageAddresses(collector func(addr uint32)) {
	if dp == nil {
		return
	}
	collector(dp.addr)
	dp.p0.collectDirPageAddresses(collector)
	for _, e := range dp.entries {
		e.p.collectDirPageAddresses(collector)
	}
}

func (dp *dirPage) collectDirEntries(collector func(entry dirEntry)) {
	if dp == nil {
		return
	}
	dp.p0.collectDirEntries(collector)
	for _, e := range dp.entries {
		collector(e)
		e.p.collectDirEntries(collector)
	}
}

func (dp *dirPage) getAddr() uint32 {
	if dp == nil {
		return 0
	}
	return dp.addr
}

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
	"time"

	"github.com/asig/odit/internal/disk"
	"github.com/asig/odit/internal/util"
)

const (
	headerMark = 0x9BA71D86

	headerSize = 352
	secTabSize = 64
	exTabSize  = 12

	ofsFilename = 4
	ofsAleng    = 36
	ofsBleng    = 38
	ofsDate     = 40
	ofsTime     = 44
	ofsExtTable = 48
	ofsSecTable = 96
)

type fileHeader disk.Sector

/*
	FileHeader* =
		RECORD (Disk.Sector)   (*allocated in the first page of each file on disk*)
			mark*: LONGINT;			// Offset: 0
			name*: FileName;		// Offset: 4
			aleng*, bleng*: INTEGER;// Offset: 36, 38
			date*, time*: LONGINT;	// Offset: 40, 44
			ext*:  ExtensionTable;	// Offset: 48
			sec*: SectorTable;		// Offset: 96
			fill: ARRAY SectorSize - HeaderSize OF CHAR;
		END ;

*/

func (f *fileHeader) IsValid() bool {
	return util.ReadLEUint32(f[:], 0) == headerMark
}

func (f *fileHeader) setMark() {
	util.WriteLEUint32(f[:], 0, headerMark)
}

func (f *fileHeader) name() string {
	return util.StringFromBytes(f[ofsFilename : ofsFilename+fnLength])
}

func (f *fileHeader) setName(name string) {
	if len(name) > fnLength {
		panic(fmt.Sprintf("name too long: %d > %d", len(name), fnLength))
	}

	util.WriteFixedLengthString(f[:], ofsFilename, fnLength, name)
}

func (f *fileHeader) aleng() uint16 {
	return util.ReadLEUint16(f[:], ofsAleng)
}

func (f *fileHeader) bleng() uint16 {
	return util.ReadLEUint16(f[:], ofsBleng)
}

func (f *fileHeader) setAleng(aleng uint16) {
	util.WriteLEUint16(f[:], ofsAleng, aleng)
}

func (f *fileHeader) setBleng(bleng uint16) {
	util.WriteLEUint16(f[:], ofsBleng, bleng)
}

func (f *fileHeader) creationTime() time.Time {
	// Oberon date/time format according to Project Oberon (1992) [https://people.inf.ethz.ch/wirth/ProjectOberon1992.pdf] :
	//
	// time = (hour*64 + min)*64 + sec
	// date = (year*16 + month)*32 + day

	// Oberon date/time format according to Native Oberon 2.3.6 source code:
	/*
		WritePair(" ", d MOD 32); WritePair(".", d DIV 32 MOD 16);
		Write(W, ".");  WriteInt(W, 1900 + d DIV 512, 1);
		WritePair(" ", t DIV 4096 MOD 32); WritePair(":", t DIV 64 MOD 64); WritePair(":", t MOD 64)
	*/

	d := util.ReadLEUint32(f[:], ofsDate)
	day := d % 32
	month := (d / 32) % 16
	year := 1900 + (d / 512)

	t := util.ReadLEUint32(f[:], ofsTime)
	sec := t % 64
	min := (t / 64) % 64
	hour := (t / 4096) % 32

	return time.Date(int(year), time.Month(month), int(day), int(hour), int(min), int(sec), 0, time.UTC)
}

func (f *fileHeader) setCreationTime(t time.Time) {
	year := uint32(t.Year() - 1900)
	month := uint32(t.Month())
	day := uint32(t.Day())
	hour := uint32(t.Hour())
	min := uint32(t.Minute())
	sec := uint32(t.Second())

	date := (year * 512) + (month * 32) + day
	time := (hour * 4096) + (min * 64) + sec

	util.WriteLEUint32(f[:], ofsDate, date)
	util.WriteLEUint32(f[:], ofsTime, time)
}

func (f *fileHeader) getExtensionTable() []uint32 {
	ext := make([]uint32, 0, exTabSize)
	for i := 0; i < exTabSize; i++ {
		adr := util.ReadLEUint32(f[:], ofsExtTable+i*4)
		if adr != 0 {
			ext = append(ext, adr)
		}
	}
	return ext
}

func (f *fileHeader) setExtensionTable(extTable []uint32) {
	for i := 0; i < exTabSize; i++ {
		addr := uint32(0)
		if i < len(extTable) {
			addr = extTable[i]
		}
		util.WriteLEUint32(f[:], int(ofsExtTable+i*4), addr)
	}
}

func (f *fileHeader) getSectorTable() []uint32 {
	sec := make([]uint32, 0, secTabSize)
	for i := 0; i < secTabSize; i++ {
		adr := util.ReadLEUint32(f[:], ofsSecTable+i*4)
		if adr != 0 {
			sec = append(sec, adr)
		}
	}
	return sec
}

func (f *fileHeader) setSectorTableEntry(index uint32, addr uint32) {
	if index >= secTabSize {
		panic(fmt.Sprintf("index out of range: %d >= %d", index, secTabSize))
	}
	util.WriteLEUint32(f[:], int(ofsSecTable+index*4), addr)
}

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

package disk

import (
	"fmt"
	"os"

	"github.com/asig/odit/internal/util"
)

const (
	SectorSize       = 2048
	SectorMultiplier = uint32(29) // Oberon sector number is multiplied by 29

	oberonPartitionType = 79 // Native Oberon partition type

	bs  = 512             // disk block size
	bps = SectorSize / bs // blocks per sector

	maxDrives     = 4
	MaxPartitions = 32
	reserved      = 32 // sectors reserved for writing during trap handling

	defaultCacheSize = 100 // default sector cache size
	cacheReserved    = 8   // cache sectors reserved for writing during trap handling
)

type Sector [SectorSize]byte

type Disk struct {
	f *os.File

	partitionOffset uint32 // partition offset in blocks
	partitionLen    uint32 // partition length in blocks
	rootOffset      uint32 // root directory offset in blocks
	nummax          uint32 // max sector number (in Oberon sectors)
}

type partition struct {
	partitionType uint8
	start         uint32
	size          uint32
}

type nodeRec struct {
	data  [SectorSize]byte
	next  *nodeRec
	adr   int64
	dirty bool
}

func Open(imagePath string) (*Disk, error) {
	f, err := os.OpenFile(imagePath, os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	disk := &Disk{f: f}
	err = disk.init()
	if err != nil {
		disk.Close()
		return nil, err
	}
	return disk, nil
}

func (d *Disk) Close() error {
	return d.f.Close()
}

// Size returns the size of the disk in "encoded" Oberon sectors.
func (d *Disk) Size() uint32 {
	return d.nummax * SectorMultiplier
}

func (d *Disk) init() error {
	/*
		PROCEDURE InitTable;
		CONST BootDiskette = 0;
		VAR s, x, pn, pi: LONGINT;  b: ARRAY BS OF CHAR;  pt: ARRAY MaxPartitions OF Partition;
		BEGIN
			native := TRUE;
			GetBlocks(BootDiskette, 0, 1, b, 0);	(* read boot block of first disk to check if diskette *)
			x := 0;  SYSTEM.GET(SYSTEM.ADR(b[510]), SYSTEM.VAL(INTEGER, x));
			b[0] := "x"; b[1] := "x"; b[2] := "x";  b[9] := 0X;
			IF (x = 0AA55H) & (b = "xxxOBERON") & (b[24H] = 0X) THEN	(* diskette with valid boot block *)
				ddrive := BootDiskette;  partitionoffset := 0;
				GetParams(BootDiskette, x, pn, pi);
				partitionlen := x * pn * pi
			ELSE	(* read partition table, finding first Native Oberon partition *)
				ReadPartitionTable(pt, pn);
				pi := 0;  x := pn;
				WHILE pi # x DO
					IF pt[pi].type = parttype THEN x := pi
					ELSE INC(pi)
					END
				END;
				IF pi = pn THEN error := "Partition not found";  ShowPartitionTable(pt, pn); RETURN END;
				partitionoffset := pt[pi].start;  partitionlen := pt[pi].size;
				ddrive := pt[pi].drive;
				GetBlocks(ddrive, partitionoffset, 1, b, 0);	(* read boot block to get offset *)
				x := 0;  SYSTEM.GET(SYSTEM.ADR(b[510]), SYSTEM.VAL(INTEGER, x));
				b[0] := "x"; b[1] := "x"; b[2] := "x";  b[9] := 0X;
				IF (x # 0AA55H) OR (b # "xxxOBERON") THEN error := "Bad boot block";  RETURN END
			END;
			rootoffset := 0;  SYSTEM.GET(SYSTEM.ADR(b[0EH]), SYSTEM.VAL(INTEGER, rootoffset));
			s := 0;  SYSTEM.GET(SYSTEM.ADR(b[13H]), SYSTEM.VAL(INTEGER, s));	(* total size *)
			IF s = 0 THEN SYSTEM.GET(SYSTEM.ADR(b[20H]), s) END;
			IF partitionlen > s THEN partitionlen := s END;	(* limit to size of file system *)
			ASSERT(partitionlen > 0);
				(* total size of file system *)
			nummaxdisk := (partitionlen-rootoffset) DIV BPS;
			nummax := nummaxdisk;
			IF writein & (Csize > nummax) THEN nummax := Csize END;	(* use the full cache *)
				(* set up sector reservation table *)
			s := (nummax+1+31) DIV 32;
			NEW(map, s);
			WHILE s # 0 DO DEC(s); map[s] := 0 END;
			INCL(SYSTEM.VAL(SET, map[0]), 0)	(* reserve sector 0 (illegal to use) *)
		END InitTable;
	*/

	// Read partition table, finding first Native Oberon partition
	partitions, err := d.readPartitionTable()
	if err != nil {
		return err
	}
	oberonPart := -1
	for i, part := range partitions {
		if part.partitionType == oberonPartitionType {
			oberonPart = i
			break
		}
	}
	if oberonPart == -1 {
		return fmt.Errorf("init: Oberon partition not found")
	}
	d.partitionOffset = partitions[oberonPart].start
	d.partitionLen = partitions[oberonPart].size

	b := make([]byte, bs)
	d.getBlocks(d.partitionOffset, 1, b, 0) // read boot block to get offset

	if util.ReadLEUint16(b, 0) != 0xAA55 {
		if !(b[3] == 'O' && b[4] == 'B' && b[5] == 'E' && b[6] == 'R' && b[7] == 'O' && b[8] == 'N') {
			return fmt.Errorf("init: bad boot block signature: %s", util.HexDump(b, 0, 9))
		}
	}

	d.rootOffset = uint32(util.ReadLEUint16(b, 0xe))
	/* Computing totalSize is somewhat not working :-/ Ignroring it for now...
	totalSize := util.ReadLEUint16(b, 0x13)
	if totalSize == 0 {
		totalSize = util.ReadLEUint16(b, 0x20)
	}
	if d.partitionLen > uint32(totalSize) {
		d.partitionLen = uint32(totalSize) // limit to size of file system
	}
	*/
	if d.partitionLen <= 0 {
		return fmt.Errorf("init: invalid partition length %d", d.partitionLen)
	}
	// total size of file system
	nummaxdisk := (d.partitionLen - d.rootOffset) / uint32(bps)
	d.nummax = nummaxdisk

	return nil
}

func (d *Disk) getBlocks(start, num uint32, buf []byte, ofs int) error {
	// log.Debug().Msgf("getBlocks: reading %d blocks starting at %d", num, start)

	b := make([]byte, num*bs)
	d.f.Seek(int64(start*bs), 0)
	count, err := d.f.Read(b)
	if err != nil {
		return err
	}
	if count < int(num*bs) {
		return fmt.Errorf("getBlocks: short read, expected %d bytes, got %d", num*bs, count)
	}
	copy(buf[ofs:], b)
	return nil
}

func (d *Disk) putBlocks(start, num uint32, buf []byte, ofs int) error {
	// log.Debug().Msgf("putBlocks: writing %d blocks starting at %d", num, start)

	b := make([]byte, num*bs)
	if len(buf[ofs:]) < int(num*bs) {
		return fmt.Errorf("putBlocks: short buffer, expected at least %d bytes, got %d", num*bs, len(buf[ofs:]))
	}
	copy(b, buf[ofs:])

	_, err := d.f.Seek(int64(start*bs), 0)
	if err != nil {
		panic(err)
	}
	_, err = d.f.Write(b)
	if err != nil {
		panic(err)
	}
	return nil
}

/*
(** GetBlocks - Read 512-byte disk blocks.  Low-level interface to driver.
	"drive" - hard disk number (0=first, 1=second, etc.)
	"start" - start sector number
	"num" - number of sectors
	"buf" - buffer to read into
	"ofs" - offset from start of buf in bytes *)
	GetBlocks*: TransferProc;
	SafeGetBlocks: TransferProc;	(* used by Ge
*/

func (d *Disk) readPartitionTable() (partitions []partition, err error) {
	parts, err := d.readPrimary()
	if err != nil {
		return nil, err
	}

	for _, part := range parts {
		if isExtended(part.partitionType) {
			logicalParts, err := d.readLogical(part.start)
			if err != nil {
				return nil, err
			}
			partitions = append(partitions, logicalParts...)
		} else {
			partitions = append(partitions, part)
		}
	}

	return partitions, nil
}

func isExtended(partitionType uint8) bool {
	return partitionType == 5 || partitionType == 15
}

func (d *Disk) readPrimary() (partitions []partition, err error) {
	// Read MBR
	b := make([]byte, bs)
	err = d.getBlocks(0, 1, b, 0)
	if err != nil {
		return nil, err
	}

	if !(b[510] == 0x55) && (b[511] == 0xAA) { // signature bad
		return nil, fmt.Errorf("readPrimary: bad MBR signature %02X %02X", b[510], b[511])
	}

	for i := 0; i < 4; i++ {
		part := partition{}

		// Partition starts at offset 0x1be + 16*i
		e := 0x1BE + 16*i
		partitionSize := util.ReadLEUint32(b, e+12)
		partitionType := b[e+4]
		if partitionType != 0x00 && partitionSize != 0 {
			part.partitionType = uint8(partitionType)
			part.start = util.ReadLEUint32(b, e+8)
			part.size = partitionSize

			partitions = append(partitions, part)
		}
	}
	return partitions, nil
}

func (d *Disk) readLogical(first uint32) (partitions []partition, err error) {
	/*
		PROCEDURE ReadLogical(d, first: LONGINT;  VAR p: ARRAY OF Partition;  VAR n, letter: LONGINT);
		VAR b: ARRAY BS OF CHAR;  e, sec, size, i: LONGINT;  found: BOOLEAN;
		BEGIN
			sec := first;
			REPEAT
				found := FALSE;
				GetBlocks(d, sec, 1, b, 0);
				IF (b[510] = 055X) & (b[511] = 0AAX) THEN
					FOR i := 0 TO 3 DO	(* look for partition entry (max one expected) *)
						e := 01BEH + 16*i;  SYSTEM.GET(SYSTEM.ADR(b[e+12]), size);
						IF (b[e+4] # 0X) & ~Extended(ORD(b[e+4])) & (size # 0) THEN
							p[n].type := ORD(b[e+4]);  p[n].drive := d;
							IF Lettered(p[n].type) THEN
								p[n].letter := CHR(letter);  INC(letter)
							ELSE
								p[n].letter := 0X
							END;
							SYSTEM.GET(SYSTEM.ADR(b[e+8]), p[n].start);  INC(p[n].start, sec);
							p[n].size := size;  INC(n)
						END
					END;
					i := 0;
					WHILE (i # 4) & ~found DO	(* look for nested extended entry (max one expected) *)
						e := 01BEH + 16*i;  SYSTEM.GET(SYSTEM.ADR(b[e+12]), size);
						IF Extended(ORD(b[e+4])) & (size # 0) THEN	(* found *)
							SYSTEM.GET(SYSTEM.ADR(b[e+8]), sec);  INC(sec, first);
							i := 4;  found := TRUE
						ELSE
							INC(i)
						END
					END
				ELSE
					WriteBadSignature(d, sec, b[510], b[511])
				END
			UNTIL ~found
		END ReadLogical;
	*/

	sec := first

	for {
		found := false
		b := make([]byte, bs)
		err := d.getBlocks(sec, 1, b, 0)
		if err != nil {
			return nil, err
		}

		if !(b[510] == 0x55) && (b[511] == 0xAA) { // signature bad
			return nil, fmt.Errorf("readLogical: bad signature at sector %d: %02X %02X", sec, b[510], b[511])
		}

		// Look for partition entry (max one expected)
		for i := 0; i < 4; i++ {
			e := 0x1BE + 16*i
			partitionSize := util.ReadLEUint32(b, e+12)
			partitionType := b[e+4]
			if partitionType != 0x00 && !isExtended(partitionType) && partitionSize != 0 {
				part := partition{}
				part.partitionType = partitionType
				part.start = util.ReadLEUint32(b, e+8) + sec
				part.size = partitionSize

				partitions = append(partitions, part)
			}
		}

		// Look for nested extended entry (max one expected)
		for i := 0; i < 4 && !found; {
			e := 0x1BE + 16*i
			partitionSize := util.ReadLEUint32(b, e+12)
			partitionType := b[e+4]
			if isExtended(partitionType) && partitionSize != 0 {
				sec = util.ReadLEUint32(b, e+8) + first
				i = 4
				found = true
			} else {
				i++
			}
		}

		if !found {
			break
		}
	}

	return partitions, nil
}

// PutSector Writes a 2048-byte Oberon sector. Sector addresses are 1-based!
// "src" is the sector number (in "encoded" Oberon sectors, i.e. multiple of 29)
// "sec" is the sector data to write.
func (d *Disk) PutSector(src uint32, sec Sector) error {
	if src%SectorMultiplier != 0 {
		panic(fmt.Sprintf("PutSector: invalid sector number %d (mod %d == %d)", src, SectorMultiplier, src%SectorMultiplier))
	}
	src = src / SectorMultiplier

	if src < 1 || src > d.nummax {
		return fmt.Errorf("PutSector: invalid sector number %d (not in 1..%d)", src, d.nummax)
	}

	return d.putBlocks(d.partitionOffset+d.rootOffset+(src-1)*bps, bps, sec[:], 0)
}

func (d *Disk) MustGetSector(src uint32) Sector {
	sec, err := d.GetSector(src)
	if err != nil {
		panic(fmt.Sprintf("MustGetSector: failed to read sector %d: %v", src, err))
	}
	return sec
}

func (d *Disk) MustPutSector(src uint32, sec Sector) {
	err := d.PutSector(src, sec)
	if err != nil {
		panic(fmt.Sprintf("MustPutSector: failed to write sector %d: %v", src, err))
	}
}

// GetSector reads a 2048-byte Oberon sector. Sector addresses are 1-based!
// src is sector number (in "encoded" Oberon sectors, i.e. multiple of 29)
// dest is the buffer to read into.
func (d *Disk) GetSector(src uint32) (Sector, error) {
	if src%SectorMultiplier != 0 {
		panic(fmt.Sprintf("GetSector: invalid sector number %d (mod %d == %d)", src, SectorMultiplier, src%SectorMultiplier))
	}
	src = src / SectorMultiplier
	if src < 1 || src > d.nummax {
		return Sector{}, fmt.Errorf("GetSector: invalid sector number %d (not in 1..%d)", src, d.nummax)
	}

	var sec Sector
	err := d.getBlocks(d.partitionOffset+d.rootOffset+(src-1)*bps, bps, sec[:], 0)
	return sec, err

	/*
		PROCEDURE GetSector*(src: LONGINT; VAR dest: Sector);
		VAR n: Node;
		BEGIN
			IF ~init OR (src MOD 29 # 0) THEN Halt(15) END;
			src := src DIV 29;
			IF (src < 1) OR (src > nummax) THEN Halt(15) END;
			INC(Creads);
			n := Find(src);
			IF n = NIL THEN	(* miss *)
				IF writein & (src > nummaxdisk) THEN	(* in virtual disk only *)
					INC(Cvirtualreads);
					ClearSector(SYSTEM.ADR(dest))
				ELSE (* in real disk *)
					IF readin THEN Halt(15) END;	(* cache was primed! *)
					IF native THEN
						SafeGetBlocks(ddrive, partitionoffset + rootoffset+(src-1)*BPS, BPS, dest, 0)
					ELSE
						SafeGetBlocks(ddrive, partitionoffset + ABS(map[src]), BPS, dest, 0)
					END;
					IF cache # NIL THEN
						n := Replace(src);
						CopySector(SYSTEM.ADR(dest), SYSTEM.ADR(n.data[0]))
					END
				END
			ELSE	(* hit *)
				INC(Creadhits);
				CopySector(SYSTEM.ADR(n.data[0]), SYSTEM.ADR(dest))
			END
		END GetSector;
	*/

}

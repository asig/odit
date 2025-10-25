package filesystem

import (
	"github.com/asig/ofs/internal/disk"
	"github.com/asig/ofs/internal/util"
)

const (
	dirPgSize    = 50
	dirEntrySize = fnLength + 8
	n            = dirPgSize / 2
)

type dirEntry struct {
	name string
	adr  uint32
	p    uint32
}

type dirPage struct {
	sector disk.Sector

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

}

func (d *dirPage) mark() uint32 {
	return util.ReadLEUint32(d.sector[:], 0)
}

func (d *dirPage) p0() uint32 {
	return util.ReadLEUint32(d.sector[:], 8)
}

func (d *dirPage) entries() []dirEntry {
	entries := make([]dirEntry, 0, dirPgSize)
	m := util.ReadLEUint16(d.sector[:], 4)
	for i := 0; i < int(m); i++ {
		offset := 48 + i*dirEntrySize
		name := util.StringFromBytes(d.sector[offset : offset+fnLength])
		adr := util.ReadLEUint32(d.sector[:], offset+fnLength)
		p := util.ReadLEUint32(d.sector[:], offset+fnLength+4)
		entries = append(entries, dirEntry{
			name: name,
			adr:  adr,
			p:    p,
		})
	}
	return entries
}

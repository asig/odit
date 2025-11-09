package filesystem

import (
	"fmt"

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

func (f *fileHeader) date() uint32 {
	return util.ReadLEUint32(f[:], ofsDate)
}

func (f *fileHeader) time() uint32 {
	return util.ReadLEUint32(f[:], ofsTime)
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

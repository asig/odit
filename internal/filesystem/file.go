package filesystem

import (
	"fmt"
	"math/rand"
	"os"

	"github.com/asig/ofs/internal/disk"
	"github.com/asig/ofs/internal/util"
)

type File struct {
	fs         *FileSystem
	header     fileHeader
	headerAddr uint32
}

func (f *File) Size() uint32 {
	return uint32(f.header.aleng())*sectorSize + uint32(f.header.bleng()) - headerSize
}

func (f *File) Name() string {
	return f.header.name()
}

func (f *File) HeaderAddr() uint32 {
	return f.headerAddr
}

func (f *File) physicalPos(p uint32) (sector, offset uint32) {
	p += headerSize // Adjust for header
	sector = p / sectorSize
	offset = p % sectorSize
	return
}

// getSectorAddr returns the disk address of the i-th sector of the file.
func (f *File) getSectorAddr(i uint32) uint32 {
	// No idea why we don't have special handling for i==0 here
	// Need to check the Oberon sources...
	if i < secTabSize {
		// Sector table
		secTable := f.header.getSectorTable()
		return secTable[i]
	}

	i -= secTabSize

	indexBlockIndex := i / indexSize
	if indexBlockIndex >= exTabSize {
		panic("file too large")
	}
	extTable := f.header.getExtensionTable()
	if len(extTable) <= int(indexBlockIndex) {
		panic(fmt.Sprintf("index block %d for file sector %d missing", indexBlockIndex, i+secTabSize+1))
	}
	indexBlock := indexSector(f.fs.disk.MustGetSector(extTable[indexBlockIndex]))
	return indexBlock.entries()[i%indexSize]
}

func (f *File) WriteAt(pos uint32, data []byte) error {
	fmt.Fprintf(os.Stderr, "CURRENT SIZE OF FILE %s: %d\n", f.Name(), f.Size())
	minSize := pos + uint32(len(data))
	f.ensureSize(minSize)

	fmt.Fprintf(os.Stderr, "CURRENT SIZE OF FILE %s: %d\n", f.Name(), f.Size())

	firstSectorIdx, firstOffset := f.physicalPos(pos)

	// Fill remaining data for first sector
	remainingInFirst := int(sectorSize - firstOffset)
	if remainingInFirst > len(data) {
		remainingInFirst = len(data)
	}
	sectorAddr := f.getSectorAddr(firstSectorIdx)
	sectorData := f.fs.disk.MustGetSector(sectorAddr)
	copy(sectorData[firstOffset:], data[:remainingInFirst])
	f.fs.disk.PutSector(sectorAddr, sectorData)
	data = data[remainingInFirst:]

	// Fill full sectors in the middle
	sectorIdx := firstSectorIdx + 1
	for len(data) >= sectorSize {
		sectorAddr := f.getSectorAddr(sectorIdx)
		sectorData := f.fs.disk.MustGetSector(sectorAddr)
		copy(sectorData[:], data[:sectorSize])
		f.fs.disk.PutSector(sectorAddr, sectorData)
		data = data[sectorSize:]
		sectorIdx++
	}

	// Fill remaining data for last sector
	if len(data) > 0 {
		sectorAddr := f.getSectorAddr(sectorIdx)
		sectorData := f.fs.disk.MustGetSector(sectorAddr)
		copy(sectorData[:], data[:])
		f.fs.disk.PutSector(sectorAddr, sectorData)
	}

	return nil
}

func (f *File) ReadAt(pos uint32, l uint32) ([]byte, error) {
	if f.Size() > pos+l {
		l = f.Size() - pos
	}
	var data []byte

	firstSectorIdx, firstOffset := f.physicalPos(pos)

	// Fill data from first sector
	sectorAddr := f.getSectorAddr(firstSectorIdx)
	sectorData := f.fs.disk.MustGetSector(sectorAddr)
	remainingInFirst := sectorSize - firstOffset
	if l < remainingInFirst {
		remainingInFirst = l
	}
	data = append(data, sectorData[firstOffset:firstOffset+remainingInFirst]...)
	l -= remainingInFirst

	// Read full sectors in the middle
	sectorIdx := firstSectorIdx + 1
	for l > sectorSize {
		sectorAddr := f.getSectorAddr(sectorIdx)
		sectorData := f.fs.disk.MustGetSector(sectorAddr)
		data = append(data, sectorData[:]...)
		l -= sectorSize
		sectorIdx++
	}

	// Get partial last sector
	if l > 0 {
		sectorAddr := f.getSectorAddr(sectorIdx)
		sectorData := f.fs.disk.MustGetSector(sectorAddr)
		data = append(data, sectorData[:l]...)
	}

	return data, nil
}

func (f *File) ensureSize(l uint32) {
	if l <= f.Size() {
		// The file is already large enough
		return
	}

	// Find current # of sectors the file occupies
	size := f.Size() + headerSize
	curSecs := (size + sectorSize - 1) / sectorSize

	// Find the requested # of sectors
	newSize := l + headerSize
	newSecs := (newSize + sectorSize - 1) / sectorSize

	// Allocate additional sectors if needed
	// TODO(asigner): Clear the data?
	for i := curSecs; i < newSecs; i++ {
		newSecAddr := f.fs.AllocSector(rand.Uint32() % uint32(f.fs.disk.Size()/disk.SectorMultiplier) * disk.SectorMultiplier)
		f.addSector(uint32(i), newSecAddr)
	}

	// Update aleng and bleng in header
	f.header.setAleng(uint16(newSize / sectorSize))
	f.header.setBleng(uint16(newSize % sectorSize))

	f.fs.disk.PutSector(f.headerAddr, disk.Sector(f.header))
}

func (f *File) addSector(index, addr uint32) {
	if index < secTabSize {
		// Sector table
		util.WriteLEUint32(f.header[:], int(96+index*4), addr)
		return
	}

	// Find correct index block
	index -= secTabSize

	indexBlockIndex := index / indexSize
	if indexBlockIndex >= exTabSize {
		panic("file too large")
	}

	var indexBlockAddr uint32
	extTable := f.header.getExtensionTable()
	for len(extTable) <= int(indexBlockIndex) {
		// Allocate new index block
		newIndexBlockAddr := f.fs.AllocSector(extTable[len(extTable)-1])
		extTable = append(extTable, newIndexBlockAddr)
		f.header.setExtensionTable(extTable)
	}
	indexBlockAddr = extTable[indexBlockIndex]

	indexBlock := indexSector(f.fs.disk.MustGetSector(indexBlockAddr))
	indexBlock.setEntry(index%indexSize, addr)
	f.fs.disk.PutSector(indexBlockAddr, disk.Sector(indexBlock))
}

func (f *File) Register() error {
	existingFile, err := f.fs.Find(f.Name())
	if err != nil {
		return fmt.Errorf("error checking existing file: %s", err)
	}
	if existingFile != nil {
		return fmt.Errorf("file %s already exists", f.Name())
	}
	err = f.fs.Insert(f)
	if err != nil {
		return fmt.Errorf("error inserting file: %s", err)
	}
	return nil
}

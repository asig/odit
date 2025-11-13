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
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/asig/odit/internal/disk"
	"github.com/asig/odit/internal/util"
	"github.com/rs/zerolog/log"
)

const (
	N = dirPgSize / 2

	// Consts from FileDir.Mod
	fnLength   = 32
	sectorSize = 2048
	dirRootAdr = 29
)

type FileSystem struct {
	disk *disk.Disk

	sectorMapMutex       sync.RWMutex
	sectorReservationMap util.BitSet
	numUsedSectors       uint32

	filesMutex sync.RWMutex
	files      []dirEntry
	dirPages   []uint32
	filesDirty bool
}

func New(d *disk.Disk) *FileSystem {
	fs := &FileSystem{
		disk:                 d,
		sectorReservationMap: util.NewBitSet(d.Size() / disk.SectorMultiplier), // For simplicity, keep it 1-based
	}
	fs.init()
	return fs
}

func (fs *FileSystem) Close() error {
	log.Debug().Msg("Closing filesystem")
	if fs.filesDirty {
		err := fs.writeDirectoryToDisk()
		if err != nil {
			return err
		}

	}
	log.Debug().Msg("Filesystem closed")

	return nil
}

func (fs *FileSystem) writeDirectoryToDisk() error {
	log.Debug().Msg("Writing directory to disk")

	fs.filesMutex.Lock()
	defer fs.filesMutex.Unlock()

	// Ensure all files are sorted by name
	sort.Slice(fs.files, func(i, j int) bool {
		return fs.files[i].name < fs.files[j].name
	})

	// Free all existing dirPages, we will rebuild the tree
	for _, addr := range fs.dirPages {
		fs.FreeSector(addr)
	}

	// dirPage Address Provider: reuse existing dirPages first.
	nextPageAddr := 0
	newDirPages := []uint32{}
	dirPageAddrProvider := func() uint32 {
		var addr uint32
		if nextPageAddr < len(fs.dirPages) {
			// Reuse existing page
			addr = fs.dirPages[nextPageAddr]
			nextPageAddr++
		} else {
			// Allocate new page
			addr = fs.AllocSector(0)
			fs.markSectorUsed(addr)
		}
		newDirPages = append(newDirPages, addr)
		return addr
	}

	// Build a new directory tree.
	filesWritten := 0
	pagesWritten := 0
	rootDir := fs.buildDirTree(
		nil,
		fs.files,
		dirPageAddrProvider,
		func(do *dirPage, de *dirEntry) { filesWritten++ },
		func(dp *dirPage) { pagesWritten++ },
	)

	sectorsFreed := len(fs.dirPages) - len(newDirPages)

	// Write root dir page to disk now
	rootDir.writeToDisk(fs.disk)
	fs.filesDirty = false
	fs.dirPages = newDirPages

	log.Info().Msgf("Wrote %d files in %d dir pages, freed %d unused dir pages", filesWritten, pagesWritten, sectorsFreed)

	return nil
}

func (fs *FileSystem) buildDirTree(parent *dirPage, entries []dirEntry, addrProvider func() uint32, fileCallback func(*dirPage, *dirEntry), pageCallback func(*dirPage)) *dirPage {
	if len(entries) == 0 {
		return nil
	}

	node := &dirPage{
		parent: parent,
		addr:   addrProvider(),
	}

	if len(entries) <= dirPgSize {
		// leaf node
		node.entries = make([]dirEntry, len(entries))
		copy(node.entries, entries)
		pageCallback(node)
		return node
	}

	// internal node: split into buckets, pick a root element for each bucket, build subtrees and node
	// we need max dirPageSize root elements, so we split into (dirPageSize+1) buckets
	nofBuckets := dirPgSize + 1
	bucketSize := (len(entries) + nofBuckets - 1) / nofBuckets
	if bucketSize < dirPgSize {
		// buckets are smaller than a dirPage: increase bucket size, reduce number of buckets
		bucketSize = dirPgSize
	}

	node.entries = make([]dirEntry, 0, dirPgSize)
	start := 0
	for i := 0; start < len(entries); i++ {
		end := start + bucketSize
		if end > len(entries) {
			end = len(entries)
		}

		bucketEntries := entries[start:end]
		if i == 0 {
			// first bucket
			node.p0 = fs.buildDirTree(node, bucketEntries, addrProvider, fileCallback, pageCallback)
		} else {
			e := bucketEntries[0]
			e.p = fs.buildDirTree(node, bucketEntries[1:], addrProvider, fileCallback, pageCallback)
			node.entries = append(node.entries, e)
			for _, e := range bucketEntries {
				fileCallback(node, &e)
			}
		}
		start = end
	}
	return node
}

func (fs *FileSystem) init() {
	fs.filesMutex.Lock()
	defer fs.filesMutex.Unlock()

	fs.sectorReservationMap.Set(0) // reserve sector 0 (illegal to use)
	fs.numUsedSectors = 0

	// Ignore existing index and scan files. Make sure that index is invalidated.
	sec := fs.disk.MustGetSector(fs.disk.Size())
	util.WriteLEUint32(sec[:], 0, 0)
	fs.disk.MustPutSector(fs.disk.Size(), sec)

	log.Info().Msg("Loading directory from disk")
	seen := make(map[uint32]struct{})
	rootDirPage, err := loadDirFromDisk(fs.disk, dirRootAdr, seen, 0)
	if err != nil {
		panic(fmt.Errorf("failed to load root dir page: %w", err))
	}
	log.Info().Msg("Directory loaded, scanning files")

	// Collect all file names and dir page addresses
	rootDirPage.collectDirEntries(func(entry dirEntry) {
		// make sure we drop the .p pointer so that we don't reference old pages
		entry.p = nil
		fs.files = append(fs.files, entry)
	})
	rootDirPage.collectDirPageAddresses(func(addr uint32) {
		fs.dirPages = append(fs.dirPages, addr)
	})
	fs.filesDirty = false

	// mark all dirPages sectors as used
	for _, addr := range fs.dirPages {
		fs.markSectorUsed(addr)
	}

	// Mark all sectors of all files as used
	for _, entry := range fs.files {
		// Add all "primary sectors"
		fh := fileHeader(fs.disk.MustGetSector(entry.adr))
		for _, secAddr := range fh.getSectorTable() {
			fs.markSectorUsed(secAddr)
		}
		// Add sectors via index tables
		for _, extAdr := range fh.getExtensionTable() {
			fs.markSectorUsed(extAdr)
			isec := indexSector(fs.disk.MustGetSector(extAdr))
			for _, dataAdr := range isec.entries() {
				fs.markSectorUsed(dataAdr)
			}
		}
	}
	log.Info().Msgf("%d files allocating %d sectors found", len(fs.files), fs.numUsedSectors)
}

func (fs *FileSystem) Find(name string) (*File, error) {
	fs.filesMutex.RLock()
	defer fs.filesMutex.RUnlock()

	return fs.find_locked(name)
}

func (fs *FileSystem) find_locked(name string) (*File, error) {
	for _, entry := range fs.files {
		if entry.name == name {
			return fs.NewFileFromFileHeader(entry.adr)
		}
	}
	return nil, nil
}

func (fs *FileSystem) Remove(name string) bool {
	fs.filesMutex.Lock()
	defer fs.filesMutex.Unlock()

	for idx, entry := range fs.files {
		if entry.name == name {
			// Remove file entry
			fs.files = append(fs.files[:idx], fs.files[idx+1:]...)
			fs.filesDirty = true
			return true
		}
	}
	return false
}

type ListFileFilter func(*File) bool

var AllFiles ListFileFilter = func(f *File) bool {
	return true
}

func (fs *FileSystem) ListFiles(pred ListFileFilter) ([]*File, error) {
	fs.filesMutex.RLock()
	defer fs.filesMutex.RUnlock()

	var files []*File
	for _, entry := range fs.files {
		f, _ := fs.NewFileFromFileHeader(entry.adr)
		if pred(f) {
			files = append(files, f)
		}
	}
	return files, nil
}

func (fs *FileSystem) IsSectorFree(addr uint32) bool {
	if addr%disk.SectorMultiplier != 0 {
		panic(fmt.Sprintf("IsSectorFree: addr not a multiple of %d", disk.SectorMultiplier))
	}
	return !fs.sectorReservationMap.Test(addr / disk.SectorMultiplier)
}

func (fs *FileSystem) FreeSector(addr uint32) {
	fs.sectorMapMutex.Lock()
	defer fs.sectorMapMutex.Unlock()

	if addr%disk.SectorMultiplier != 0 {
		panic(fmt.Sprintf("FreeSector: addr not a multiple of %d", disk.SectorMultiplier))
	}
	fs.sectorReservationMap.Clear(addr / disk.SectorMultiplier)
	fs.numUsedSectors--
}

func (fs *FileSystem) markSectorUsed(addr uint32) {
	fs.sectorMapMutex.Lock()
	defer fs.sectorMapMutex.Unlock()

	if addr%disk.SectorMultiplier != 0 {
		panic(fmt.Sprintf("markSectorUsed: addr not a multiple of %d", disk.SectorMultiplier))
	}
	fs.sectorReservationMap.Set(addr / disk.SectorMultiplier)
	fs.numUsedSectors++
}

// AllocSector allocates a new sector. "hint" can be previously allocated
// sector to preserve adjacency, or 0 if previous sector not known.
func (fs *FileSystem) AllocSector(hint uint32) uint32 {
	fs.sectorMapMutex.Lock()
	defer fs.sectorMapMutex.Unlock()

	if hint%disk.SectorMultiplier != 0 {
		panic(fmt.Sprintf("AllocSector: hint not a multiple of %d", disk.SectorMultiplier))
	}

	if hint > fs.disk.Size() {
		hint = 0
	}
	sec := hint + 29
	for {
		if sec == hint {
			panic("Disk full")
		}
		if fs.IsSectorFree(sec) {
			fs.sectorReservationMap.Set(sec / disk.SectorMultiplier)
			fs.numUsedSectors++
			return sec
		}
		sec += disk.SectorMultiplier
		if sec > fs.disk.Size() {
			sec = 29
		}
	}
}

func (fs *FileSystem) NewFileFromFileHeader(headerAddr uint32) (*File, error) {
	header := fileHeader(fs.disk.MustGetSector(headerAddr))
	if !header.IsValid() {
		return nil, fmt.Errorf("invalid file header")
	}

	return &File{
		header:     header,
		headerAddr: headerAddr,
		fs:         fs,
	}, nil
}

func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func (fs *FileSystem) validateFilename(name string) error {
	if len(name) > fnLength {
		return fmt.Errorf("file name too long: %d > %d", len(name), fnLength)
	}
	if len(name) == 0 {
		return fmt.Errorf("file name cannot be empty")
	}
	// Must start with a letter
	if !isLetter(name[0]) {
		return fmt.Errorf("file name must start with a letter")
	}
	// Only allow letters, digits, dot
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !isLetter(c) && !isDigit(c) && c != '.' {
			return fmt.Errorf("file name contains invalid character: %c", c)
		}
	}
	return nil
}

func (fs *FileSystem) NewFile(name string) (*File, error) {
	if err := fs.validateFilename(name); err != nil {
		return nil, err
	}
	fileHeader := fileHeader{}
	headerAddr := fs.AllocSector(rand.Uint32() % uint32(fs.disk.Size()/disk.SectorMultiplier) * disk.SectorMultiplier)
	fileHeader.setMark()
	fileHeader.setName(name)
	fileHeader.setAleng(0)
	fileHeader.setBleng(headerSize)
	fileHeader.setSectorTableEntry(0, headerAddr)
	fileHeader.setCreationTime(time.Now())
	fs.disk.PutSector(headerAddr, disk.Sector(fileHeader))

	return &File{
		header:     fileHeader,
		headerAddr: headerAddr,
		fs:         fs,
	}, nil
}

func (fs *FileSystem) Insert(f *File) error {
	// Check if the file already exists
	name := f.header.name()
	existing, err := fs.Find(name)
	if err != nil {
		return err
	}
	if existing != nil {
		return fmt.Errorf("File %s already exists", name)
	}

	fs.files = append(fs.files, dirEntry{
		name: name,
		adr:  f.headerAddr,
		p:    nil,
	})

	fs.filesDirty = true
	return nil
}

package filesystem

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/asig/odit/internal/disk"
	"github.com/asig/odit/internal/util"
	"github.com/rs/zerolog/log"
)

const (
	N = dirPgSize / 2

	// Consts from FileDir.Mod
	fnLength     = 32
	sectorSize   = 2048
	dirRootAdr   = 29
	fillerSize   = 36
	mapIndexSize = (sectorSize - 4) / 4
	mapSize      = sectorSize / 4 // {MapSize MOD 32 = 0}
	mapMark      = 0x9C2F977F
)

type FileSystem struct {
	disk                 *disk.Disk
	sectorReservationMap util.BitSet
	numUsedSectors       uint32

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
	if fs.filesDirty {
		err := fs.writeDirectoryToDisk()
		if err != nil {
			return err
		}

	}

	return nil
}

func (fs *FileSystem) writeDirectoryToDisk() error {
	log.Info().Msg("Writing directory to disk")

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

/*
		MapIndex =
			RECORD (Disk.Sector)
				mark: LONGINT;
				index: ARRAY MapIndexSize OF DiskAdr
			END ;

		MapSector =
			RECORD (Disk.Sector)
				map: ARRAY MapSize OF SET
			END ;

	VAR
		prefix*: ARRAY 10 OF CHAR;
		PathChar*: CHAR;
		pat: ARRAY 32 OF CHAR;
		pos: INTEGER;
		init: BOOLEAN;
		hp: POINTER TO FileHeader;	(* ptr so as not to take inner core space *)

	PROCEDURE AddStr(s1, s2: ARRAY OF CHAR;  VAR s3: ARRAY OF CHAR);
		VAR i, j, l: LONGINT;
	BEGIN j := 0; l := LEN(s3)-1; i := 0;
		WHILE (s1[i] # 0X) & (j # l) DO s3[j] := s1[i]; INC(i); INC(j) END;
		i := 0;
		WHILE (s2[i] # 0X) & (j # l) DO s3[j] := s2[i]; INC(i); INC(j) END;
		s3[j] := 0X
	END AddStr;

	(*Exported procedures: Search, Insert, Delete, Enumerate, Init*)

	PROCEDURE Search*(VAR name: FileName; VAR A: DiskAdr);
		VAR i, L, R: INTEGER; dadr: DiskAdr;
			a: DirPage; n: FileName; first: BOOLEAN;
	BEGIN IF ~init THEN HALT(99) END; first := TRUE;
		LOOP
			IF first THEN AddStr(prefix, name, n) ELSE COPY(name, n) END;
			dadr := DirRootAdr;
			LOOP Disk.GetSector(dadr, a);
				L := 0; R := a.m; (*binary search*)
				WHILE L < R DO
					i := (L+R) DIV 2;
					IF n <= a.e[i].name THEN R := i ELSE L := i+1 END
				END ;
				IF (R < a.m) & (n = a.e[R].name) THEN
					A := a.e[R].adr; EXIT (*found*)
				END ;
				IF R = 0 THEN dadr := a.p0 ELSE dadr := a.e[R-1].p END ;
				IF dadr = 0 THEN A := 0; EXIT  (*not found*) END
			END;
			IF (A # 0) OR ~first OR (prefix = "") THEN EXIT END;
			first := FALSE
		END
	END Search;

	PROCEDURE insert(VAR name: FileName;
									 dpg0:  DiskAdr;
									 VAR h: BOOLEAN;
									 VAR v: DirEntry;
									 fad:     DiskAdr);
		(*h = "tree has become higher and v is ascending element"*)
		VAR ch: CHAR;
			i, j, L, R: INTEGER;
			dpg1: DiskAdr;
			u: DirEntry;
			a: DirPage;

	BEGIN (*~h*) Disk.GetSector(dpg0, a);
		L := 0; R := a.m; (*binary search*)
		WHILE L < R DO
			i := (L+R) DIV 2;
			IF name <= a.e[i].name THEN R := i ELSE L := i+1 END
		END ;
		IF (R < a.m) & (name = a.e[R].name) THEN
			a.e[R].adr := fad; Disk.PutSector(dpg0, a)  (*replace*)
		ELSE (*not on this page*)
			IF R = 0 THEN dpg1 := a.p0 ELSE dpg1 := a.e[R-1].p END ;
			IF dpg1 = 0 THEN (*not in tree, insert*)
				u.adr := fad; u.p := 0; h := TRUE; j := 0;
				REPEAT ch := name[j]; u.name[j] := ch; INC(j)
				UNTIL ch = 0X;
				WHILE j < FnLength DO u.name[j] := 0X; INC(j) END
			ELSE
				insert(name, dpg1, h, u, fad)
			END ;
			IF h THEN (*insert u to the left of e[R]*)
				IF a.m < DirPgSize THEN
					h := FALSE; i := a.m;
					WHILE i > R DO DEC(i); a.e[i+1] := a.e[i] END ;
					a.e[R] := u; INC(a.m)
				ELSE (*split page and assign the middle element to v*)
					a.m := N; a.mark := DirMark;
					IF R < N THEN (*insert in left half*)
						v := a.e[N-1]; i := N-1;
						WHILE i > R DO DEC(i); a.e[i+1] := a.e[i] END ;
						a.e[R] := u; Disk.PutSector(dpg0, a);
						Disk.AllocSector(dpg0, dpg0); i := 0;
						WHILE i < N DO a.e[i] := a.e[i+N]; INC(i) END
					ELSE (*insert in right half*)
						Disk.PutSector(dpg0, a);
						Disk.AllocSector(dpg0, dpg0); DEC(R, N); i := 0;
						IF R = 0 THEN v := u
						ELSE v := a.e[N];
							WHILE i < R-1 DO a.e[i] := a.e[N+1+i]; INC(i) END ;
							a.e[i] := u; INC(i)
						END ;
						WHILE i < N DO a.e[i] := a.e[N+i]; INC(i) END
					END ;
					a.p0 := v.p; v.p := dpg0
				END ;
				Disk.PutSector(dpg0, a)
			END
		END
	END insert;


	PROCEDURE enumerate(VAR prefix:   ARRAY OF CHAR;
											dpg:          DiskAdr;
											detail: BOOLEAN;
											proc:         EntryHandler;
											VAR continue: BOOLEAN);
		VAR i, j, diff: INTEGER; dpg1: DiskAdr;
				a: DirPage;  time, date, size: LONGINT;
	BEGIN Disk.GetSector(dpg, a); i := 0;
		WHILE (i < a.m) & continue DO
			j := 0;
			LOOP
				IF prefix[j] = 0X THEN diff := 0; EXIT END ;
				diff := ORD(a.e[i].name[j]) - ORD(prefix[j]);
				IF diff # 0 THEN EXIT END ;
				INC(j)
			END ;
			IF i = 0 THEN dpg1 := a.p0 ELSE dpg1 := a.e[i-1].p END ;
			IF diff >= 0 THEN (*matching prefix*)
				IF dpg1 # 0 THEN enumerate(prefix, dpg1, detail, proc, continue) END ;
				IF diff = 0 THEN
					IF continue & ((pos = -1) OR match(a.e[i].name)) THEN
						IF detail THEN
							Disk.GetSector(a.e[i].adr, hp^);
							time := hp.time;  date := hp.date;
							size := LONG(hp.aleng)*SectorSize + hp.bleng - HeaderSize
						ELSE
							time := 0; date := 0; size := MIN(LONGINT)
						END;
						proc(a.e[i].name, time, date, size, continue)
					END
				ELSE continue := FALSE
				END
			END ;
			INC(i)
		END ;
		IF continue & (i > 0) & (a.e[i-1].p # 0) THEN
			enumerate(prefix, a.e[i-1].p, detail, proc, continue)
		END
	END enumerate;

	PROCEDURE Enumerate*(mask: ARRAY OF CHAR; detail: BOOLEAN; proc: EntryHandler);
		VAR b: BOOLEAN;
	BEGIN
		IF ~init THEN HALT(99) END;
		COPY(mask, pat);
		pos := 0;  WHILE (pat[pos] # 0X) & (pat[pos] # "*") DO INC(pos) END;
		IF pat[pos] # "*" THEN	(* no * found *)
			pos := -1
		ELSIF (pat[pos] = "*") & (pat[pos+1] = 0X) THEN	(* found * at end *)
			mask[pos] := 0X;  pos := -1
		ELSE
			mask[pos] := 0X
		END;
		b := TRUE; enumerate(mask, DirRootAdr, detail, proc, b)
	END Enumerate;

	PROCEDURE ^Startup;

	PROCEDURE Init*;
		VAR k: INTEGER;
				A: ARRAY 2000 OF DiskAdr;
				files: LONGINT;  bad: BOOLEAN;

		PROCEDURE MarkSectors;
			VAR L, R, i, j, n: INTEGER; x: DiskAdr;
				hd: FileHeader;
				B: IndexSector;

			PROCEDURE sift(L, R: INTEGER);
				VAR i, j: INTEGER; x: DiskAdr;
			BEGIN j := L; x := A[j];
				LOOP i := j; j := 2*j + 1;
					IF (j+1 < R) & (A[j] < A[j+1]) THEN INC(j) END ;
					IF (j >= R) OR (x > A[j]) THEN EXIT END ;
					A[i] := A[j]
				END ;
				A[i] := x
			END sift;

		BEGIN
			Kernel.WriteString(" marking");
			L := k DIV 2; R := k; (*heapsort*)
			WHILE L > 0 DO DEC(L); sift(L, R) END ;
			WHILE R > 0 DO
				DEC(R); x := A[0]; A[0] := A[R]; A[R] := x; sift(L, R)
			END;
			WHILE L < k DO
				bad := FALSE; INC(files);
				IF files MOD 128 = 0 THEN Kernel.WriteChar(".") END;
				Disk.GetSector(A[L], hd);
				IF hd.aleng < SecTabSize THEN j := hd.aleng + 1;
					REPEAT
						DEC(j);
						IF hd.sec[j] # 0 THEN Disk.MarkSector(hd.sec[j]) ELSE hd.aleng := j-1; bad := TRUE END
					UNTIL j = 0
				ELSE
					j := SecTabSize;
					REPEAT
						DEC(j);
						IF hd.sec[j] # 0 THEN Disk.MarkSector(hd.sec[j]) ELSE hd.aleng := j-1; bad := TRUE END
					UNTIL j = 0;
					n := (hd.aleng - SecTabSize) DIV IndexSize; i := 0;
					WHILE (i <= n) & ~bad DO
						IF hd.ext[i] # 0 THEN
							Disk.MarkSector(hd.ext[i]);
							Disk.GetSector(hd.ext[i], B); (*index sector*)
							IF i < n THEN j := IndexSize ELSE j := (hd.aleng - SecTabSize) MOD IndexSize + 1 END ;
							REPEAT
								DEC(j);
								IF (B.x[j] MOD 29 = 0) & (B.x[j] > 0) THEN Disk.MarkSector(B.x[j]) ELSE j := 0; bad := TRUE END
							UNTIL j = 0;
							INC(i)
						ELSE bad := TRUE
						END;
						IF bad THEN
							IF i = 0 THEN hd.aleng := SecTabSize-1 ELSE hd.aleng := SecTabSize + (i-1) * IndexSize END
						END
					END
				END;
				IF bad THEN
					Kernel.WriteLn; Kernel.WriteString(hd.name); Kernel.WriteString(" truncated");
					hd.bleng := SectorSize;  IF hd.aleng < 0 THEN hd.aleng := 0 (* really bad *) END;
					Disk.PutSector(A[L], hd)
				END;
				INC(L)
			END
		END MarkSectors;

		PROCEDURE TraverseDir(dpg: DiskAdr);
			VAR i: INTEGER; a: DirPage;
		BEGIN Disk.GetSector(dpg, a); Disk.MarkSector(dpg); i := 0;
			WHILE i < a.m DO
				A[k] := a.e[i].adr; INC(k); INC(i);
				IF k = 2000 THEN MarkSectors; k := 0 END
			END ;
			IF a.p0 # 0 THEN
				TraverseDir(a.p0); i := 0;
				WHILE i < a.m DO
					TraverseDir(a.e[i].p); INC(i)
				END
			END
		END TraverseDir;

	BEGIN
		IF ~init THEN
			Disk.ResetDisk; k := 0;
			Startup;
			IF ~init THEN
				files := 0;  Kernel.WriteString("FileDir: Scanning...");
				TraverseDir(DirRootAdr); MarkSectors; init := TRUE;
				Kernel.WriteInt(files, 6); Kernel.WriteString(" files");  Kernel.WriteLn
			END
		END
	END Init;

PROCEDURE Startup;
VAR
	j, sec, size, q, free, thres: LONGINT;  mi: MapIndex;  ms: MapSector;
	s: ARRAY 10 OF CHAR;  found: BOOLEAN;
BEGIN
	size := Disk.Size();  found := FALSE;
	IF (Disk.Available() = size) & (size # 0) THEN	(* all sectors available *)
		Disk.GetSector(size*29, mi);
		IF mi.mark = MapMark THEN
			j := 0;	(* check consistency of index *)
			WHILE (j # MapIndexSize) & (mi.index[j] >= 0) & (mi.index[j] MOD 29 = 0) DO
				INC(j)
			END;
			IF j = MapIndexSize THEN
				found := TRUE;
				mi.mark := 0;  Disk.PutSector(size*29, mi);	(* invalidate index *)
				j := 0;  sec := 1;  q := 0;
				LOOP
					IF (j = MapIndexSize) OR (mi.index[j] = 0) THEN EXIT END;
					Disk.GetSector(mi.index[j], ms);
					REPEAT
						IF (sec MOD 32) IN ms.map[sec DIV 32 MOD MapSize] THEN
							Disk.MarkSector(sec*29);
							INC(q)
						END;
						IF sec = size THEN EXIT END;
						INC(sec)
					UNTIL sec MOD (MapSize*32) = 0;
					INC(j)
				END;
				Kernel.GetConfig("DiskGC", s);
				thres := 0;  j := 0;
				WHILE s[j] # 0X DO thres := thres*10+(ORD(s[j])-48); INC(j) END;
				IF thres < 10 THEN thres := 10
				ELSIF thres > 100 THEN thres := 100
				END;
				ASSERT(q = size-Disk.Available());
				free := Disk.Available()*100 DIV size;
				IF (free > thres) & (Disk.Available()*SectorSize > 100000H) THEN
					init := TRUE
				ELSE	(* undo *)
					FOR j := 29 TO size*29 BY 29 DO
						IF Disk.Marked(j) THEN Disk.FreeSector(j) END
					END;
					ASSERT(Disk.Available() = size);
					Kernel.WriteString("FileDir: ");  Kernel.WriteInt(free, 1);
					Kernel.WriteString("% free, forcing disk GC");  Kernel.WriteLn
				END
			END
		END;
		IF ~found THEN Kernel.WriteString("FileDir: Index not found");  Kernel.WriteLn END
	END
END Startup;

*/

func (fs *FileSystem) Find(name string) (*File, error) {
	e, err := fs.findFile(func(entry dirEntry) bool {
		return entry.name == name
	})
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, nil
	}
	return fs.NewFileFromFileHeader(e.adr)
}

func (fs *FileSystem) findFile(pred func(dirEntry) bool) (e *dirEntry, err error) {
	for _, entry := range fs.files {
		if pred(entry) {
			return &entry, nil
		}
	}
	return nil, nil
}

type ListFileFilter func(*File) bool

var AllFiles ListFileFilter = func(f *File) bool {
	return true
}

func (fs *FileSystem) ListFiles(pred ListFileFilter) ([]*File, error) {
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
	if addr%disk.SectorMultiplier != 0 {
		panic(fmt.Sprintf("FreeSector: addr not a multiple of %d", disk.SectorMultiplier))
	}
	fs.sectorReservationMap.Clear(addr / disk.SectorMultiplier)
	fs.numUsedSectors--
}

func (fs *FileSystem) markSectorUsed(addr uint32) {
	if addr%disk.SectorMultiplier != 0 {
		panic(fmt.Sprintf("markSectorUsed: addr not a multiple of %d", disk.SectorMultiplier))
	}
	fs.sectorReservationMap.Set(addr / disk.SectorMultiplier)
	fs.numUsedSectors++
}

// AllocSector allocates a new sector. "hint" can be previously allocated
// sector to preserve adjacency, or 0 if previous sector not known.
func (fs *FileSystem) AllocSector(hint uint32) uint32 {
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

/*
	IF a.m < DirPgSize THEN
		h := FALSE; i := a.m;
		WHILE i > R DO DEC(i); a.e[i+1] := a.e[i] END ;
		a.e[R] := u; INC(a.m)
	ELSE (*split page and assign the middle element to v*)
		a.m := N; a.mark := DirMark;
		IF R < N THEN (*insert in left half*)
			v := a.e[N-1]; i := N-1;
			WHILE i > R DO DEC(i); a.e[i+1] := a.e[i] END ;
			a.e[R] := u; Disk.PutSector(dpg0, a);
			Disk.AllocSector(dpg0, dpg0); i := 0;
			WHILE i < N DO a.e[i] := a.e[i+N]; INC(i) END
		ELSE (*insert in right half*)
			Disk.PutSector(dpg0, a);
			Disk.AllocSector(dpg0, dpg0); DEC(R, N); i := 0;
			IF R = 0 THEN v := u
			ELSE v := a.e[N];
				WHILE i < R-1 DO a.e[i] := a.e[N+1+i]; INC(i) END ;
				a.e[i] := u; INC(i)
			END ;
			WHILE i < N DO a.e[i] := a.e[N+i]; INC(i) END
		END ;
		a.p0 := v.p; v.p := dpg0
	END ;
	Disk.PutSector(dpg0, a)
*/

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

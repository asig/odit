package filesystem

import (
	"bytes"
	"fmt"

	"github.com/asig/ofs/internal/disk"
	"github.com/asig/ofs/internal/util"
)

const (
	// Consts from FileDir.Mod
	fnLength     = 32
	secTabSize   = 64
	exTabSize    = 12
	sectorSize   = 2048
	indexSize    = sectorSize / 4
	headerSize   = 352
	dirRootAdr   = 29
	dirMark      = 0x9B1EA38D
	headerMark   = 0x9BA71D86
	fillerSize   = 36
	mapIndexSize = (sectorSize - 4) / 4
	mapSize      = sectorSize / 4 // {MapSize MOD 32 = 0}
	mapMark      = 0x9C2F977F
)

type FileSystem struct {
	disk                 *disk.Disk
	sectorReservationMap util.BitSet
}

func New(d *disk.Disk) *FileSystem {
	fs := &FileSystem{
		disk:                 d,
		sectorReservationMap: util.NewBitSet(int(d.Size()/disk.SectorSize) + 1), // For simplicity, keep it 1-based
	}
	fs.init()
	return fs
}

func (fs *FileSystem) init() {
	fs.sectorReservationMap.Set(0) // reserve sector 0 (illegal to use)

	// Ignore existing index and scan files. Make sure that index is invalidated.
	sec := fs.disk.MustGetSector(fs.disk.Size())
	util.WriteLEUint32(sec[:], 0, 0)
	fs.disk.MustPutSector(fs.disk.Size(), sec)
}

type fileHeader struct {
	sector disk.Sector

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
}

func (f *fileHeader) mark() uint32 {
	return util.ReadLEUint32(f.sector[:], 0)
}

func (f *fileHeader) name() string {
	nameBytes := f.sector[4 : 4+fnLength]
	// Trim at first null byte
	return string(bytes.TrimRight(nameBytes, "\x00"))
}

func (f *fileHeader) aleng() uint16 {
	return util.ReadLEUint16(f.sector[:], 36)
}

func (f *fileHeader) bleng() uint16 {
	return util.ReadLEUint16(f.sector[:], 38)
}

func (f *fileHeader) date() uint32 {
	return util.ReadLEUint32(f.sector[:], 40)
}

func (f *fileHeader) time() uint32 {
	return util.ReadLEUint32(f.sector[:], 44)
}

func (f *fileHeader) getExtensionTable() []uint32 {
	ext := make([]uint32, 0, exTabSize)
	for i := 0; i < exTabSize; i++ {
		adr := util.ReadLEUint32(f.sector[:], 48+i*4)
		if adr != 0 {
			ext = append(ext, adr)
		}
	}
	return ext
}

func (f *fileHeader) getSectorTable() []uint32 {
	sec := make([]uint32, 0, secTabSize)
	for i := 0; i < secTabSize; i++ {
		adr := util.ReadLEUint32(f.sector[:], 96+i*4)
		if adr != 0 {
			sec = append(sec, adr)
		}
	}
	return sec
}

/* ** The FileDir module implements the naming of files in directories. *)

	(*File Directory is a B-tree with its root page at DirRootAdr.
		Each entry contains a file name and the disk address of the file's head sector*)

	CONST FnLength*    = 32;
				SecTabSize*   = 64;
				ExTabSize*   = 12;
				SectorSize*   = 2048;	(* Disk.SectorSize *)
				IndexSize*   = SectorSize DIV 4;
				HeaderSize*  = 352;
				DirRootAdr*  = 29;
				DirPgSize*   = 50;
				N = DirPgSize DIV 2;
				DirMark*    = 9B1EA38DH;
				HeaderMark* = 9BA71D86H;
				FillerSize = 36;
				MapIndexSize = (SectorSize-4) DIV 4;
				MapSize = SectorSize DIV 4;	(* {MapSize MOD 32 = 0} *)
				MapMark = 9C2F977FH;

	TYPE
		DiskAdr      = LONGINT;
		FileName*       = ARRAY FnLength OF CHAR;
		SectorTable*    = ARRAY SecTabSize OF DiskAdr;
		ExtensionTable* = ARRAY ExTabSize OF DiskAdr;
(* An EntryHandler is used by the Enumerate operation.  name contains the name of the file.
time, date and size are only used if the detail flag was specified in Enumerate.  continue may
be set to FALSE to stop the Enumerate operation mid-way. *)
		EntryHandler* = PROCEDURE (name: ARRAY OF CHAR; time, date, size: LONGINT; VAR continue: BOOLEAN);

		FileHeader* =
			RECORD (Disk.Sector)   (*allocated in the first page of each file on disk*)
				mark*: LONGINT;
				name*: FileName;
				aleng*, bleng*: INTEGER;
				date*, time*: LONGINT;
				ext*:  ExtensionTable;
				sec*: SectorTable;
				fill: ARRAY SectorSize - HeaderSize OF CHAR;
			END ;

		IndexSector* =
			RECORD (Disk.Sector)
				x*: ARRAY IndexSize OF DiskAdr
			END ;

		DataSector* =
			RECORD (Disk.Sector)
				B*: ARRAY SectorSize OF SYSTEM.BYTE
			END ;

		DirEntry* =  (*B-tree node*)
			RECORD
				name*: FileName;
				adr*:  DiskAdr; (*sec no of file header*)
				p*:    DiskAdr  (*sec no of descendant in directory*)
			END ;

		DirPage*  =
			RECORD (Disk.Sector)
				mark*:  LONGINT;	// Offset: 0
				m*:     INTEGER;	// Offset: 4
				p0*:    DiskAdr;    // Offset: 8(*sec no of left descendant in directory*)
				fill:   ARRAY FillerSize OF CHAR; // Offset: 12
				e*:  ARRAY DirPgSize OF DirEntry	// Offset: 48
			END ;

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

	PROCEDURE Insert*(VAR name: FileName; fad: DiskAdr);
		VAR  oldroot: DiskAdr;
			h: BOOLEAN; U: DirEntry;
			a: DirPage; n: FileName;
	BEGIN IF ~init THEN HALT(99) END;
		h := FALSE; AddStr(prefix, name, n);
		insert(n, DirRootAdr, h, U, fad);
		IF h THEN (*root overflow*)
			Disk.GetSector(DirRootAdr, a);
			Disk.AllocSector(DirRootAdr, oldroot); Disk.PutSector(oldroot, a);
			a.mark := DirMark; a.m := 1; a.p0 := oldroot; a.e[0] := U;
			Disk.PutSector(DirRootAdr, a)
		END
	END Insert;


	PROCEDURE underflow(VAR c: DirPage;  (*ancestor page*)
											dpg0:  DiskAdr;
											s:     INTEGER;  (*insertion point in c*)
											VAR h: BOOLEAN); (*c undersize*)
		VAR i, k: INTEGER;
				dpg1: DiskAdr;
				a, b: DirPage;  (*a := underflowing page, b := neighbouring page*)
	BEGIN Disk.GetSector(dpg0, a);
		(*h & a.m = N-1 & dpg0 = c.e[s-1].p*)
		IF s < c.m THEN (*b := page to the right of a*)
			dpg1 := c.e[s].p; Disk.GetSector(dpg1, b);
			k := (b.m-N+1) DIV 2; (*k = no. of items available on page b*)
			a.e[N-1] := c.e[s]; a.e[N-1].p := b.p0;
			IF k > 0 THEN
				(*move k-1 items from b to a, one to c*) i := 0;
				WHILE i < k-1 DO a.e[i+N] := b.e[i]; INC(i) END ;
				c.e[s] := b.e[i]; b.p0 := c.e[s].p;
				c.e[s].p := dpg1; DEC(b.m, k); i := 0;
				WHILE i < b.m DO b.e[i] := b.e[i+k]; INC(i) END ;
				Disk.PutSector(dpg1, b); a.m := N-1+k; h := FALSE
			ELSE (*merge pages a and b, discard b*) i := 0;
				WHILE i < N DO a.e[i+N] := b.e[i]; INC(i) END ;
				i := s; DEC(c.m);
				WHILE i < c.m DO c.e[i] := c.e[i+1]; INC(i) END ;
				a.m := 2*N; h := c.m < N
			END ;
			Disk.PutSector(dpg0, a)
		ELSE (*b := page to the left of a*) DEC(s);
			IF s = 0 THEN dpg1 := c.p0 ELSE dpg1 := c.e[s-1].p END ;
			Disk.GetSector(dpg1, b);
			k := (b.m-N+1) DIV 2; (*k = no. of items available on page b*)
			IF k > 0 THEN
				i := N-1;
				WHILE i > 0 DO DEC(i); a.e[i+k] := a.e[i] END ;
				i := k-1; a.e[i] := c.e[s]; a.e[i].p := a.p0;
				(*move k-1 items from b to a, one to c*) DEC(b.m, k);
				WHILE i > 0 DO DEC(i); a.e[i] := b.e[i+b.m+1] END ;
				c.e[s] := b.e[b.m]; a.p0 := c.e[s].p;
				c.e[s].p := dpg0; a.m := N-1+k; h := FALSE;
				Disk.PutSector(dpg0, a)
			ELSE (*merge pages a and b, discard a*)
				c.e[s].p := a.p0; b.e[N] := c.e[s]; i := 0;
				WHILE i < N-1 DO b.e[i+N+1] := a.e[i]; INC(i) END ;
				b.m := 2*N; DEC(c.m); h := c.m < N
			END ;
			Disk.PutSector(dpg1, b)
		END
	END underflow;

	PROCEDURE delete(VAR name: FileName;
									 dpg0: DiskAdr;
									 VAR h: BOOLEAN;
									 VAR fad: DiskAdr);
	(*search and delete entry with key name; if a page underflow arises,
		balance with adjacent page or merge; h := "page dpg0 is undersize"*)

		VAR i, L, R: INTEGER;
			dpg1: DiskAdr;
			a: DirPage;

		PROCEDURE del(dpg1: DiskAdr; VAR h: BOOLEAN);
			VAR dpg2: DiskAdr;  (*global: a, R*)
					b: DirPage;
		BEGIN Disk.GetSector(dpg1, b); dpg2 := b.e[b.m-1].p;
			IF dpg2 # 0 THEN del(dpg2, h);
				IF h THEN underflow(b, dpg2, b.m, h); Disk.PutSector(dpg1, b) END
			ELSE
				b.e[b.m-1].p := a.e[R].p; a.e[R] := b.e[b.m-1];
				DEC(b.m); h := b.m < N; Disk.PutSector(dpg1, b)
			END
		END del;

	BEGIN (*~h*) Disk.GetSector(dpg0, a);
		L := 0; R := a.m; (*binary search*)
		WHILE L < R DO
			i := (L+R) DIV 2;
			IF name <= a.e[i].name THEN R := i ELSE L := i+1 END
		END ;
		IF R = 0 THEN dpg1 := a.p0 ELSE dpg1 := a.e[R-1].p END ;
		IF (R < a.m) & (name = a.e[R].name) THEN
			(*found, now delete*) fad := a.e[R].adr;
			IF dpg1 = 0 THEN  (*a is a leaf page*)
				DEC(a.m); h := a.m < N; i := R;
				WHILE i < a.m DO a.e[i] := a.e[i+1]; INC(i) END
			ELSE del(dpg1, h);
				IF h THEN underflow(a, dpg1, R, h) END
			END ;
			Disk.PutSector(dpg0, a)
		ELSIF dpg1 # 0 THEN
			delete(name, dpg1, h, fad);
			IF h THEN underflow(a, dpg1, R, h); Disk.PutSector(dpg0, a) END
		ELSE (*not in tree*) fad := 0
		END
	END delete;

	PROCEDURE Delete*(VAR name: FileName; VAR fad: DiskAdr);
		VAR h: BOOLEAN; newroot: DiskAdr;
			a: DirPage; n: FileName;
	BEGIN IF ~init THEN HALT(99) END;
		h := FALSE; AddStr(prefix, name, n);
		delete(n, DirRootAdr, h, fad);
		IF h THEN (*root underflow*)
			Disk.GetSector(DirRootAdr, a);
			IF (a.m = 0) & (a.p0 # 0) THEN
				newroot := a.p0; Disk.GetSector(newroot, a);
				Disk.PutSector(DirRootAdr, a) (*discard newroot*)
			END
		END
	END Delete;

	PROCEDURE match(VAR name: ARRAY OF CHAR): BOOLEAN;
	VAR i0, i1, j0, j1: INTEGER;  f: BOOLEAN;
	BEGIN
		i0 := pos;  j0 := pos;  f := TRUE;
		LOOP
			IF pat[i0] = "*" THEN
				INC(i0);
				IF pat[i0] = 0X THEN EXIT END
			ELSE
				IF name[j0] # 0X THEN f := FALSE END;
				EXIT
			END;
			f := FALSE;
			LOOP
				IF name[j0] = 0X THEN EXIT END;
				i1 := i0;  j1 := j0;
				LOOP
					IF (pat[i1] = 0X) OR (pat[i1] = "*") THEN f := TRUE; EXIT END;
					IF pat[i1] # name[j1] THEN EXIT END;
					INC(i1);  INC(j1)
				END;
				IF f THEN j0 := j1; i0 := i1; EXIT END;
				INC(j0)
			END;
			IF ~f THEN EXIT END
		END;
		RETURN f & (name[0] # 0X)
	END match;

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

PROCEDURE *Cleanup;
VAR i, j, p, q, sec, size: LONGINT;  mi: MapIndex;  ms: MapSector;
BEGIN
	size := Disk.Size();  i := size*29;
	IF ~Disk.Marked(i) THEN	(* last sector is free *)
		j := 0;  sec := 1;  q := 0;
		LOOP
			REPEAT DEC(i, 29) UNTIL (i = 0) OR ~Disk.Marked(i);	(* find a free sector *)
			IF i = 0 THEN RETURN END;	(* no more space, don't commit *)
			mi.index[j] := i;  INC(j);
			FOR p := 0 TO MapSize-1 DO ms.map[p] := {} END;
			REPEAT
				IF Disk.Marked(sec*29) THEN
					INCL(ms.map[sec DIV 32 MOD MapSize], sec MOD 32);
					INC(q)
				END;
				IF sec = size THEN
					Disk.PutSector(i, ms);
					EXIT
				END;
				INC(sec)
			UNTIL sec MOD (MapSize*32) = 0;
			Disk.PutSector(i, ms)
		END;
		WHILE j # MapIndexSize DO mi.index[j] := 0; INC(j) END;
		mi.mark := MapMark;
		Disk.PutSector(size*29, mi);	(* commit *)
		Kernel.WriteString("FileDir: Map saved");  Kernel.WriteLn
	END
END Cleanup;

BEGIN
	Kernel.GetConfig("Prefix", prefix);  init := FALSE;  PathChar := "/";  NEW(hp);
	Kernel.InstallTermHandler(Cleanup)
END FileDir.
*/

func (fs *FileSystem) collectFiles(addr uint32) ([]dirEntry, error) {
	var entries []dirEntry

	data, err := fs.disk.GetSector(addr)
	if err != nil {
		return nil, fmt.Errorf("Error reading sector %d: %s\n", addr, err)
	}

	dpage := dirPage{
		sector: data,
	}

	p0 := dpage.p0()
	if p0 != 0 {
		de, err := fs.collectFiles(p0)
		if err != nil {
			return nil, err
		}
		entries = append(entries, de...)
	}

	for _, de := range dpage.entries() {
		entries = append(entries, de)
		if de.p != 0 {
			de, err := fs.collectFiles(de.p)
			if err != nil {
				return nil, err
			}
			entries = append(entries, de...)
		}
	}
	return entries, nil
}

type FileInfo struct {
	Name string
	Adr  uint32
	Size uint32
}

func (fs *FileSystem) getFileInfo(adr uint32) (*FileInfo, error) {
	sec, err := fs.disk.GetSector(adr)
	if err != nil {
		return nil, fmt.Errorf("Error reading sector %d: %s\n", adr, err)
	}
	fh := fileHeader{
		sector: sec,
	}
	return &FileInfo{
		Name: fh.name(),
		Adr:  adr,
		Size: uint32(fh.aleng())*sectorSize + uint32(fh.bleng()) - headerSize,
	}, nil
}

func (fs *FileSystem) ListFiles() ([]FileInfo, error) {
	entries, err := fs.collectFiles(dirRootAdr)
	if err != nil {
		return nil, fmt.Errorf("Error collecting files: %s\n", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		fi, err := fs.getFileInfo(entry.adr)
		if err != nil {
			fmt.Printf("Error getting file info for %s: %s\n", entry.name, err)
			// return nil, fmt.Errorf("Error getting file info for %s: %s\n", entry.name, err)
		} else {
			files = append(files, *fi)
		}
	}
	return files, nil
}

func (fs *FileSystem) findFile(pred func(dirEntry) bool) (e *dirEntry, err error) {
	entries, err := fs.collectFiles(dirRootAdr)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if pred(entry) {
			return &entry, nil
		}
	}
	return nil, nil
}

func (fs *FileSystem) FindFile(name string) (*FileInfo, error) {
	e, err := fs.findFile(func(entry dirEntry) bool {
		return entry.name == name
	})
	if err != nil {
		fmt.Printf("Error collecting files: %s\n", err)
		return nil, err
	}
	if e == nil {
		return nil, nil
	}
	fi, err := fs.getFileInfo(e.adr)
	if err != nil {
		return nil, fmt.Errorf("Error getting file info for %d: %s\n", e.adr, err)
	}
	return fi, nil
}

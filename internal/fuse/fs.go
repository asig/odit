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
 * along with Oberon Disk Image Tool.  If not, see <https://www.gnu.org/licenses/>.
 */

package fuse

import (
	"context"
	"os"
	"syscall"

	fuse "bazil.org/fuse"
	fuse_fs "bazil.org/fuse/fs"
	"github.com/rs/zerolog/log"

	"github.com/asig/odit/internal/filesystem"
)

type FS struct {
	fs  *filesystem.FileSystem
	uid uint32
	gid uint32
}

type dirNode struct {
	fs  *filesystem.FileSystem
	uid uint32
	gid uint32
}

type fileNode struct {
	file *filesystem.File
	uid  uint32
	gid  uint32
}

type fileHandle struct {
	file *fileNode
}

func NewFS(fs *filesystem.FileSystem) fuse_fs.FS {
	return FS{
		fs:  fs,
		uid: uint32(os.Getuid()),
		gid: uint32(os.Getgid()),
	}
}

func (f FS) Root() (fuse_fs.Node, error) {
	return &dirNode{fs: f.fs, uid: f.uid, gid: f.gid}, nil
}

func (d dirNode) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1
	a.Mode = os.ModeDir | 0755
	a.Uid = d.uid
	a.Gid = d.gid
	return nil
}

func (d dirNode) Lookup(ctx context.Context, name string) (fuse_fs.Node, error) {
	log.Debug().Msgf("FUSE Lookup for %s", name)
	file, err := d.fs.Find(name)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, syscall.ENOENT
	}

	return &fileNode{file: file, uid: d.uid, gid: d.gid}, nil
}

func (d dirNode) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	log.Debug().Msgf("FUSE ReadDirAll")
	var res []fuse.Dirent
	entries, err := d.fs.ListFiles(filesystem.AllFiles)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		res = append(res, fuse.Dirent{
			Name: entry.Name(),
			Type: fuse.DT_File,
		})
	}
	return res, nil
}

func (d dirNode) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fuse_fs.Node, fuse_fs.Handle, error) {
	log.Debug().Msgf("FUSE Create for %s", req.Name)

	file, err := d.fs.Find(req.Name)
	if err != nil {
		return nil, nil, err
	}
	if file != nil {
		return nil, nil, syscall.EEXIST
	}

	f, err := d.fs.NewFile(req.Name)
	if err != nil {
		return nil, nil, err
	}
	f.Register()

	node := fileNode{file: f, uid: d.uid, gid: d.gid}
	handle := fileHandle{file: &node}
	return node, handle, nil
}

func (d dirNode) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	log.Debug().Msgf("FUSE Remove for %s", req.Name)

	f, err := d.fs.Find(req.Name)
	if err != nil {
		return err
	}
	if f == nil {
		return syscall.ENOENT
	}

	d.fs.Remove(req.Name)
	return nil
}

func (f fileNode) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Debug().Msgf("FUSE Attr for file %s", f.file.Name())
	a.Inode = uint64(f.file.HeaderAddr())
	a.Mode = 0666 // read-only
	a.Size = uint64(f.file.Size())
	creationTime := f.file.CreationTime()
	a.Ctime = creationTime
	a.Mtime = creationTime
	a.Atime = creationTime
	a.Uid = f.uid
	a.Gid = f.gid
	return nil
}

func (f fileNode) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fuse_fs.Handle, error) {
	log.Debug().Msgf("FUSE Open for file %s: req = %+v", f.file.Name(), req)
	return fileHandle{file: &f}, nil
}

func (h fileHandle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	log.Debug().Msgf("FUSE Read for file %s: offset = %d, size = %d", h.file.file.Name(), req.Offset, req.Size)
	if req.Offset >= int64(h.file.file.Size()) {
		log.Debug().Msgf("FUSE Read for file %s: offset beyond EOF, returning empty data", h.file.file.Name())
		resp.Data = []byte{}
		return nil
	}
	if req.Offset+int64(req.Size) > int64(h.file.file.Size()) {
		log.Debug().Msgf("FUSE Read for file %s: adjusting read size to avoid EOF", h.file.file.Name())
		req.Size = int(h.file.file.Size() - uint32(req.Offset))
		log.Debug().Msgf("FUSE Read for file %s: new size = %d", h.file.file.Name(), req.Size)
	}
	buf, err := h.file.file.ReadAt(uint32(req.Offset), uint32(req.Size))
	if err != nil {
		return err
	}
	resp.Data = buf
	return nil
}

func (h fileHandle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	log.Debug().Msgf("FUSE Write for file %s: req = %+v", h.file.file.Name(), req)
	h.file.file.WriteAt(uint32(req.Offset), req.Data)
	resp.Size = len(req.Data)
	return nil
}

func (h fileHandle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	return nil
}

func (h fileHandle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	return nil
}

package fuse

import (
	"context"
	"os"
	"syscall"

	fuse "bazil.org/fuse"
	fuse_fs "bazil.org/fuse/fs"

	"github.com/asig/ofs/internal/filesystem"
	"github.com/rs/zerolog/log"
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

func (f fileNode) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Debug().Msgf("FUSE Attr for file %s", f.file.Name())
	a.Inode = uint64(f.file.HeaderAddr())
	a.Mode = 0666 // read-only
	a.Size = uint64(f.file.Size())
	a.Uid = f.uid
	a.Gid = f.gid
	return nil
}

func (f fileNode) ReadAll(ctx context.Context) ([]byte, error) {
	log.Debug().Msgf("FUSE ReadAll for file %s", f.file.Name())
	buf, err := f.file.ReadAt(0, f.file.Size())
	if err != nil {
		return nil, err
	}
	return buf, nil
}

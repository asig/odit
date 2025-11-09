package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	bazil_fuse "bazil.org/fuse"
	bazil_fuse_fs "bazil.org/fuse/fs"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/asig/ofs/internal/disk"
	"github.com/asig/ofs/internal/filesystem"
	"github.com/asig/ofs/internal/fuse"
)

const (
	version = "v0.1"
)

var (
	flagImage    = flag.String("image", "", "Image to work on")
	flagLogLevel = newLogLevelFlag(zerolog.ErrorLevel, "log-level", "Log level (trace, debug, info, warn, error, fatal, panic)")
)

func newLogLevelFlag(value zerolog.Level, name string, usage string) *logLevelFlag {
	p := &logLevelFlag{level: value}
	flag.Var(p, name, usage)
	return p
}

// logLevelFlag implements flag.Value for zerolog.Level
type logLevelFlag struct {
	level zerolog.Level
}

func (f *logLevelFlag) String() string {
	return f.level.String()
}

func (f *logLevelFlag) Set(value string) error {
	level, err := zerolog.ParseLevel(strings.ToLower(value))
	if err != nil {
		return err
	}
	f.level = level
	return nil
}

func (f *logLevelFlag) Get() zerolog.Level {
	return f.level
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s -image <image> [flags] {command}

Flags:  
   -image <image>
       Specifies the image to work on

   -log-level <level>
       Sets the log level (trace, debug, info, warn, error, fatal, panic)
	   Default is 'error'
       
Commands:
   help:
	   Shows this help message

   list:
       Lists files in the image   

   info <file>:
       Shows information about <file> in the image

   read <src> <dest>:
       Copies file from <src> in the image to <dest> on host's file system

   write <src> <dest>:
       Copies file from <src> on host's file system to <dest> in the image

   mount <mountpoint>:
       Mounts the image at <mountpoint> using FUSE; does not return until unmounted
`, os.Args[0])
	os.Exit(1)
}

func readFromImage(fs *filesystem.FileSystem, src, dest string) {
	fmt.Fprintf(os.Stderr, "read not implemented yet 😢\n")
	// TODO(asginer): Implement read command
}

func writeToImage(fs *filesystem.FileSystem, src, dest string) {
	fmt.Fprintf(os.Stderr, "write not implemented yet 😢\n")
	// TODO(asginer): Implement write command
}

func listFiles(fs *filesystem.FileSystem) {
	entries, err := fs.ListFiles(filesystem.AllFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing files: %s\n", err)
		return
	}
	for _, de := range entries {
		fmt.Println(de.Name())
	}
}

func fileInfo(fs *filesystem.FileSystem, file string) {
	f, err := fs.Find(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting file info: %s\n", err)
		return
	}
	if f == nil {
		fmt.Fprintf(os.Stderr, "File not found: %s\n", file)
		return
	}
	fmt.Printf("File: %s\n", f.Name())
	fmt.Printf("Address: %d\n", f.HeaderAddr())
	fmt.Printf("Size: %d bytes\n", f.Size())
}

func initLogging(level zerolog.Level) {
	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339Nano // Need to keep this, or we won't get millis, no matter what we say in TimeFormat below?
	log.Logger = zerolog.
		New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "2006-01-02T15:04:05.000Z07:00", // "RFC3339Millis"
			NoColor:    false,
		}).
		With().Timestamp().Caller().
		Logger()

}

func mount(fs *filesystem.FileSystem, mountpoint string) {
	fmt.Printf("Mounting image to %s...\n", mountpoint)

	// FUSE-Verbindung aufbauen
	c, err := bazil_fuse.Mount(
		mountpoint,
		bazil_fuse.FSName("Native Oberon FS"),
		bazil_fuse.Subtype("native-oberon-fs"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error mounting FUSE filesystem: %s\n", err)
		return
	}
	defer c.Close()

	// Server starten
	fmt.Printf("Image available at %s, unmount to continue.\n", mountpoint)
	err = bazil_fuse_fs.Serve(c, fuse.NewFS(fs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error serving FUSE filesystem: %s\n", err)
	}
}

func main() {
	fmt.Printf("Oberon Disk Image Tool %s\n", version)
	fmt.Printf("Copyright (c) 2025 Andreas Signer <asigner@gmail.com>\n")
	fmt.Printf("https://github.com/asig/odit\n")

	flag.Usage = usage
	flag.Parse()

	initLogging(flagLogLevel.Get())

	if *flagImage == "" {
		fmt.Fprintf(os.Stderr, "no image specified\n")
		usage()
		os.Exit(1)
	}

	disk, err := disk.Open(*flagImage)
	if err != nil {
		log.Error().Err(err).Msg("Can't open image")
		os.Exit(1)
	}
	defer disk.Close()

	fs := filesystem.New(disk)
	defer fs.Close()

	args := flag.Args()
	pos := 0
	for pos < len(args) {
		switch args[pos] {
		case "mount":
			pos++
			if pos >= len(args) {
				fmt.Fprintf(os.Stderr, "not enough arguments for mount command. Format is \"mount <mountpoint>\"\n")
				os.Exit(1)
			}
			mountpoint := args[pos]
			pos++
			mount(fs, mountpoint)
		case "list":
			pos++
			listFiles(fs)
		case "info":
			pos++
			if pos >= len(args) {
				fmt.Fprintf(os.Stderr, "not enough arguments for info command. Format is \"info <file>\"\n")
				os.Exit(1)
			}
			file := args[pos]
			pos++
			fileInfo(fs, file)
		case "read":
			pos++
			if pos+2 > len(args) {
				fmt.Fprintf(os.Stderr, "not enough arguments for read command. Format is \"read <src> <dest>\"\n")
				os.Exit(1)
			}
			src := args[pos]
			dest := args[pos+1]
			pos += 2
			readFromImage(fs, src, dest)
		case "write":
			pos++
			if pos+2 > len(args) {
				fmt.Fprintf(os.Stderr, "not enough arguments for write command. Format is \"write <src> <dest>\"\n")
				os.Exit(1)
			}
			src := args[pos]
			dest := args[pos+1]
			pos += 2
			writeToImage(fs, src, dest)
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[pos])
			usage()
			os.Exit(1)
		}
	}
}

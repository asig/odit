package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/asig/ofs/internal/disk"
	"github.com/asig/ofs/internal/filesystem"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	version = "v0.1"
)

var (
	flagImage    = flag.String("image", "", "Image to work on")
	flagLogLevel = newLogLevelFlag(zerolog.InfoLevel, "log-level", "Log level (trace, debug, info, warn, error, fatal, panic)")
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
	fmt.Fprintf(os.Stderr, `Usage: %s -image <image> [command]

Commands:
   list: Lists files in the image
   info <file>: Shows information about <file> in the image
   read <src> <dest>: Copies file from <src> in the image to <dest> on host's file system
   write <src> <dest>: Copies file from <src> on host's file system to <dest> in the image
`, os.Args[0])
	os.Exit(1)
}

func readFromImage(fs *filesystem.FileSystem, src, dest string) {
	// TODO: Implement read command
}

func writeToImage(fs *filesystem.FileSystem, src, dest string) {
	// TODO: Implement write command
}

func listFiles(fs *filesystem.FileSystem) {
	entries, err := fs.ListFiles(filesystem.AllFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing files: %s\n", err)
		return
	}
	for _, de := range entries {
		fmt.Printf("%s (%d bytes)\n", de.Name(), de.Size())
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

func test(fs *filesystem.FileSystem) {
	fmt.Printf("Running demo test...\n")
	disk, err := disk.Open("disk.img")
	if err != nil {
		log.Error().Err(err).Msg("Can't open image")
		return
	}
	defer disk.Close()

	entries, err := fs.ListFiles(filesystem.AllFiles)
	if err != nil {
		log.Error().Err(err).Msg("Can't list files")
		return
	}
	for _, de := range entries {
		log.Info().Msgf("Found file: %s (%d bytes)", de.Name(), de.Size())
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

	f, err := fs.NewFile("XBM2.txt")
	if err != nil {
		log.Err(err).Msgf("Failed to create new file: %v", err)
		os.Exit(1)
	}
	err = f.WriteAt(0, []byte("DAS IST EIN TEST VON AUSSERHALB!!!!"))
	if err != nil {
		log.Err(err).Msgf("Failed to write to new file: %v", err)
		os.Exit(1)
	}
	err = f.Register()
	if err != nil {
		log.Err(err).Msgf("Failed to register new file: %v", err)
		os.Exit(1)
	}

	args := flag.Args()
	pos := 0
	for pos < len(args) {
		switch args[pos] {
		case "DEMO":
			test(fs)
		case "list":
			pos++
			listFiles(fs)
		case "info":
			pos++
			if pos >= len(args) {
				usage()
				os.Exit(1)
			}
			file := args[pos]
			pos++
			fileInfo(fs, file)
		case "read":
			pos++
			if pos+2 >= len(args) {
				usage()
				os.Exit(1)
			}
			src := args[pos]
			dest := args[pos+1]
			readFromImage(fs, src, dest)
		case "write":
			pos++
			if pos+2 >= len(args) {
				usage()
				os.Exit(1)
			}
			src := args[pos]
			dest := args[pos+1]
			writeToImage(fs, src, dest)
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[pos])
			usage()
			os.Exit(1)
		}
	}
}

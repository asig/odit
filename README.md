# odit - Oberon Disk Image Tool

A command-line tool for working with Native Oberon disk images. `odit` allows you to list, read, write, and mount Native Oberon file systems on modern operating systems.

## Features

- **List files** in Oberon disk images
- **Read files** from Oberon images to your host file system
- **Write files** from your host file system to Oberon images
- **Mount** Oberon images as FUSE filesystems for direct file access
- **File information** display (size, creation time, disk location)

## Installation

### Prerequisites

- Go 1.25.1 or later
- FUSE libraries (for mount functionality)
  - **Linux**: `libfuse-dev` or `fuse3-dev`
  - **macOS**: Install [macFUSE](https://osxfuse.github.io/)

### Build from Source

```bash
git clone https://github.com/asig/odit.git
cd odit
go build
```

## Usage

```bash
odit -image <image_file> [flags] {command}
```

### Flags

- `-image <image>` - **Required**: Specifies the Oberon disk image to work on
- `-log-level <level>` - Sets the log level (trace, debug, info, warn, error, fatal, panic). Default: `error`

### Commands

#### List Files

List all files in the image:

```bash
odit -image disk.img list
```

#### File Information

Show detailed information about a specific file:

```bash
odit -image disk.img info System.Tool
```

Output includes:
- File name
- First block address
- File size in bytes
- Creation timestamp

#### Read File

Copy a file from the Oberon image to your host file system:

```bash
odit -image disk.img read System.Tool output.txt
```

#### Write File

Copy a file from your host file system to the Oberon image:

```bash
odit -image disk.img write input.txt NewFile.Tool
```

**Note**: File names in Oberon must:
- Start with a letter
- Contain only letters, digits, and dots
- Be 32 characters or less

#### Mount Filesystem

Mount the Oberon image at a mountpoint using FUSE:

```bash
odit -image disk.img mount /mnt/oberon
```

The filesystem will remain mounted until you unmount it:

```bash
# Linux
umount /mnt/oberon

# macOS
diskutil unmount /mnt/oberon
```

While mounted, you can access files using standard tools (`ls`, `cat`, `cp`, etc.).

## Examples

### Backup files from an Oberon image

```bash
# List all files
odit -image oberon.img list > files.txt

# Read each file
while read filename; do
    odit -image oberon.img read "$filename" "backup/$filename"
done < files.txt
```

### Browse an image with FUSE

```bash
# Mount the image
mkdir /tmp/oberon
odit -image oberon.img mount /tmp/oberon

# Browse in another terminal
cd /tmp/oberon
ls -l
cat System.Tool

# Unmount when done
umount /tmp/oberon
```

### Add files to an image

```bash
# Write a new file
echo "Hello from modern OS!" > greeting.txt
odit -image oberon.img write greeting.txt Greeting.Text

# Verify it was written
odit -image oberon.img info Greeting.Text
```

## License

Copyright (C) 2025 Andreas Signer <asigner@gmail.com>

This program is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

See the [GNU General Public License](https://www.gnu.org/licenses/) for more details.

## Links

- [Project Oberon (2005 Edition)](https://people.inf.ethz.ch/wirth/ProjectOberon1992.pdf)
- [Native Oberon](https://github.com/asig/native-oberon)
- [GitHub Repository](https://github.com/asig/odit)
# AVIF Converter

A fast, parallel image-to-AVIF converter written in Go. Convert your JPG, PNG, WebP, HEIC, BMP, and TIFF images to the modern AVIF format with significant file size reductions.

## Features

- **Parallel Processing**: Convert multiple images simultaneously with configurable worker threads
- **Multiple Format Support**: Handles JPG, JPEG, PNG, WebP, HEIC, BMP, and TIFF
- **Flexible Naming**: Keep original filenames or generate timestamped names with optional prefixes
- **Dry Run Mode**: Preview conversions before executing
- **Collision Handling**: Automatically handles duplicate output filenames
- **Progress Reporting**: Real-time conversion status and file size comparisons
- **Cross-Platform**: Works on Linux, macOS, and Windows (with prerequisites)

## Prerequisites

Before running the program, you must install `libavif-bin` which provides the `avifenc` encoder:

**Ubuntu/Debian:**
```bash
apt install libavif-bin
```

## Installation

Build the program from source:

```bash
go build -o avif-converter
```

This creates an executable named `avif-converter` in your current directory.

## Usage

### Basic Examples

**List available image types in a directory:**
```bash
./avif-converter -input ./input/photos/ -list
```

**Convert all JPG files in current directory:**
```bash
./avif-converter -format jpg
```

**Convert PNG files from a specific directory:**
```bash
./avif-converter -input ./input/photos/ -format png
```

**Convert with custom output directory:**
```bash
./avif-converter -input ./input/photos/ -format jpg -output ./output/
```

**Keep original filenames (only change extension):**
```bash
./avif-converter -input ./input/photos/ -format jpg -keep-name
```

**Add a prefix to output files:**
```bash
./avif-converter -input ./input/photos/ -format jpg -prefix vacation2024
```

**Dry run (preview without converting):**
```bash
./avif-converter -input ./input/photos/ -format jpg -dry-run
```

**Use more parallel workers for faster conversion:**
```bash
./avif-converter -input ./input/photos/ -format jpg -workers 8
```

**Complete example with all common options:**
```bash
./avif-converter -input ./input/photos/ -format jpg -output ./output/ -workers 8 -prefix vacation2024
```

### Command-Line Flags

```
-input string
    Directory to scan for image files (default ".")
-format string
    Image format to convert (e.g., jpg, jpeg, heic)  <-- required
-prefix string
    Optional prefix for output filenames
-output string
    Output directory (default: same as input)
-workers int
    Number of parallel conversion workers (default 4)
-list
    Only list available file types without converting
-dry-run
    Show what would be converted without actual conversion
-keep-name
    Keep original filename (only change extension)
```

## Output Naming

The program supports two naming modes:

**Default mode** (timestamp + random):
```
20250915_a3f2c1.avif
vacation_20250915_a3f2c1.avif  (with prefix)
```

**Keep-name mode** (`-keep-name`):
```
photo.avif
vacation_photo.avif  (with prefix)
```

If a filename collision occurs, the program automatically appends `-1`, `-2`, etc.

## Conversion Settings

The program uses these `avifenc` parameters for optimal quality and compression:
- `--min 0 --max 20`: Quality range (0 = lossless, 63 = worst)
- `--depth 10`: 10-bit color depth

These settings provide excellent quality with significant file size reduction (typically 50-80% smaller than JPEG).

## Performance Tips

- Increase `-workers` on systems with more CPU cores (try 8-16 workers)
- Use `-dry-run` first to verify your settings before converting
- Use `-list` to quickly see what image types are available
- Keep source and output directories on the same filesystem for faster atomic operations

## Supported Image Formats

- **JPG/JPEG**: Standard JPEG images
- **PNG**: Portable Network Graphics
- **WebP**: Google's WebP format
- **HEIC**: High Efficiency Image Container (Apple format)
- **BMP**: Windows Bitmap
- **TIFF**: Tagged Image File Format
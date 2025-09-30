package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type FileInfo struct {
	Path    string
	ModTime time.Time
	Ext     string // normalized ext, e.g. "jpg", "png"
	Size    int64
}

type ConvertResult struct {
	Src       string
	Dst       string
	OrigBytes int64
	ConvBytes int64
	Err       error
}

func main() {
	// CLI flags
	input := flag.String("input", ".", "Directory to scan for image files")
	format := flag.String("format", "", "Image format to convert (e.g., jpg, jpeg, heic)")
	prefix := flag.String("prefix", "", "Optional prefix for output filenames")
	output := flag.String("output", "", "Output directory (default: same as input)")
	workers := flag.Int("workers", 4, "Number of parallel conversion workers")
	listOnly := flag.Bool("list", false, "Only list available file types without converting")
	dryRun := flag.Bool("dry-run", false, "Show what would be converted without actual conversion")
	keepName := flag.Bool("keep-name", false, "Keep original filename (only change extension)")
	flag.Parse()

	// Validate input directory
	if *input == "" {
		log.Fatalf("input directory is empty")
	}
	if info, err := os.Stat(*input); err != nil || !info.IsDir() {
		log.Fatalf("input is not a directory or not accessible: %s", *input)
	}

	// Normalize requested format
	if *format != "" {
		*format = strings.ToLower(strings.TrimPrefix(*format, "."))
		if *format == "jpeg" {
			*format = "jpg"
		}
	}

	// Scan directory
	files, counts, err := scanDirectory(*input)
	if err != nil {
		log.Fatalf("scan error: %v", err)
	}

	// Print found file types
	fmt.Println("=== Found file types ===")
	for ext, cnt := range counts {
		fmt.Printf("%s: %d\n", ext, cnt)
	}

	if *listOnly {
		return
	}

	// Require format
	if *format == "" {
		fmt.Println("\nNo format specified. Use -format flag (e.g. -format jpg).")
		return
	}

	// Check that specified format exists among scanned files
	if c, ok := counts[*format]; !ok || c == 0 {
		log.Fatalf("No .%s files found in directory %s", *format, *input)
	}

	// Choose output directory
	outDir := *output
	if outDir == "" {
		outDir = *input
	}
	if err := os.MkdirAll(outDir, 0700); err != nil {
		log.Fatalf("failed to create output directory: %v", err)
	}

	// Build list to convert
	var toConvert []FileInfo
	for _, f := range files {
		if f.Ext == *format {
			toConvert = append(toConvert, f)
		}
	}

	fmt.Printf("\nConverting %d .%s files to AVIF (workers=%d)\n", len(toConvert), *format, *workers)
	if *dryRun {
		fmt.Println("DRY RUN - no conversion will be performed")
		for _, f := range toConvert {
			outName := makeOutputFilename(f, *prefix, *keepName)
			fmt.Printf("%s -> %s (%.2f MB)\n", f.Path, filepath.Join(outDir, outName), float64(f.Size)/(1024*1024))
		}
		return
	}

	// Check avifenc in PATH
	if _, err := exec.LookPath("avifenc"); err != nil {
		log.Fatalf("avifenc not found in PATH: %v", err)
	}

	// Start worker pool
	results := make(chan ConvertResult, len(toConvert))
	var wg sync.WaitGroup
	jobCh := make(chan FileInfo, len(toConvert))

	// Start workers
	numWorkers := *workers
	if numWorkers <= 0 {
		numWorkers = 1
	}
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(&wg, jobCh, results, outDir, *prefix, *keepName)
	}

	// Feed jobs
	go func() {
		for _, f := range toConvert {
			jobCh <- f
		}
		close(jobCh)
	}()

	// Close results when done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect
	var success, fail int
	var totalOrig, totalConv int64
	for r := range results {
		if r.Err != nil {
			fail++
			fmt.Fprintf(os.Stderr, "ERROR: %s -> %v\n", r.Src, r.Err)
		} else {
			success++
			fmt.Printf("OK: %s -> %s (%.2f MB -> %.2f MB, %.1f%% reduction)\n",
				filepath.Base(r.Src),
				filepath.Base(r.Dst),
				float64(r.OrigBytes)/(1024*1024),
				float64(r.ConvBytes)/(1024*1024),
				reductionPercent(r.OrigBytes, r.ConvBytes))
			totalOrig += r.OrigBytes
			totalConv += r.ConvBytes
		}
	}

	// Summary
	fmt.Printf("\nSummary: %d successful, %d failed\n", success, fail)
	if success > 0 && totalOrig > 0 {
		fmt.Printf("Total size: %.2f MB -> %.2f MB (%.1f%% reduction)\n",
			float64(totalOrig)/(1024*1024),
			float64(totalConv)/(1024*1024),
			reductionPercent(totalOrig, totalConv))
	}
}

// scanDirectory walks the inputDir and returns files and a map of counts by extension (normalized).
// Note: this function treats "jpeg" as "jpg".
func scanDirectory(inputDir string) ([]FileInfo, map[string]int, error) {
	var out []FileInfo
	counts := map[string]int{}

	extAllowed := map[string]bool{
		"jpg":  true,
		"jpeg": true,
		"png":  true,
		"bmp":  true,
		"tiff": true,
		"webp": true,
		"heic": true, // will be included but we won't parse EXIF/time specially
	}

	err := filepath.WalkDir(inputDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// don't panic; skip and log
			log.Printf("skip %s: %v", path, walkErr)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
		if ext == "" {
			return nil
		}
		if !extAllowed[ext] {
			return nil
		}
		// normalize jpeg -> jpg
		if ext == "jpeg" {
			ext = "jpg"
		}
		info, err := d.Info()
		if err != nil {
			log.Printf("can't stat %s: %v", path, err)
			return nil
		}
		fi := FileInfo{
			Path:    path,
			ModTime: info.ModTime(), // use ModTime only (cross-platform safe)
			Ext:     ext,
			Size:    info.Size(),
		}
		out = append(out, fi)
		counts[ext]++
		return nil
	})
	return out, counts, err
}

// makeOutputFilename builds filename for output. If keepName is true, preserve base name but change ext.
// Otherwise: [prefix_]?YYYYMMDD_hex.avif  (hex is short random 6 hex chars)
func makeOutputFilename(f FileInfo, prefix string, keepName bool) string {
	if keepName {
		base := filepath.Base(f.Path)
		nameNoExt := strings.TrimSuffix(base, filepath.Ext(base))
		if prefix != "" {
			return fmt.Sprintf("%s_%s.avif", prefix, nameNoExt)
		}
		return fmt.Sprintf("%s.avif", nameNoExt)
	}

	// random 3 bytes -> 6 hex chars
	rb := make([]byte, 3)
	_, err := rand.Read(rb)
	var rnd string
	if err == nil {
		rnd = hex.EncodeToString(rb)
	} else {
		// fallback
		rnd = fmt.Sprintf("%x", time.Now().UnixNano()&0xfffffff)
	}

	dateStr := f.ModTime.Format("20060102")
	if prefix != "" {
		return fmt.Sprintf("%s_%s_%s.avif", prefix, dateStr, rnd)
	}
	return fmt.Sprintf("%s_%s.avif", dateStr, rnd)
}

func worker(wg *sync.WaitGroup, jobs <-chan FileInfo, results chan<- ConvertResult, outDir, prefix string, keepName bool) {
	defer wg.Done()
	for fi := range jobs {
		res := ConvertResult{Src: fi.Path, OrigBytes: fi.Size}
		outName := makeOutputFilename(fi, prefix, keepName)
		outPath := filepath.Join(outDir, outName)

		// ensure unique path (avoid overwrite)
		uniquePath, err := ensureUniquePath(outPath)
		if err != nil {
			res.Err = fmt.Errorf("unique output path fail: %v", err)
			results <- res
			continue
		}
		res.Dst = uniquePath

		// create temp file in same directory as final output for atomic rename where possible
		tmpFile, err := os.CreateTemp(filepath.Dir(uniquePath), "avif_tmp_*.avif")
		if err != nil {
			res.Err = fmt.Errorf("create temp file: %v", err)
			results <- res
			continue
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		// tmpRemoved tracks whether we already removed the tmp file
		tmpRemoved := false
		removeTmp := func() {
			if !tmpRemoved {
				_ = os.Remove(tmpPath)
				tmpRemoved = true
			}
		}

		// run avifenc
		cmd := exec.Command("avifenc", "--min", "0", "--max", "20", "--depth", "10", fi.Path, tmpPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			removeTmp()
			res.Err = fmt.Errorf("avifenc failed: %v; output: %s", err, string(out))
			results <- res
			continue
		}

		// try rename; fallback to copy if cross-device
		if err := os.Rename(tmpPath, uniquePath); err != nil {
			if cerr := copyFile(tmpPath, uniquePath); cerr != nil {
				removeTmp()
				res.Err = fmt.Errorf("save output failed: rename: %v, copy: %v", err, cerr)
				results <- res
				continue
			}
			// copy succeeded, remove tmp
			_ = os.Remove(tmpPath)
			tmpRemoved = true
		} else {
			// rename succeeded: tmp no longer exists
			tmpRemoved = true
		}

		// stat converted
		if st, err := os.Stat(uniquePath); err == nil {
			res.ConvBytes = st.Size()
		} else {
			res.Err = fmt.Errorf("stat output failed: %v", err)
			results <- res
			continue
		}
		res.Err = nil
		results <- res
	}
}

// ensureUniquePath returns a path that does not yet exist by appending -1, -2, ... if necessary.
func ensureUniquePath(path string) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path, nil
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	for i := 1; i < 1000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s-%d%s", name, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find unique name for %s", path)
}

// copyFile copies src->dst (simple, does not preserve all metadata)
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Sync()
		out.Close()
	}()
	_, err = io.Copy(out, in)
	return err
}

func reductionPercent(orig, conv int64) float64 {
	if orig == 0 {
		return 0.0
	}
	return (1.0 - float64(conv)/float64(orig)) * 100.0
}

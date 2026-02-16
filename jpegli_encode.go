package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/gen2brain/jpegli"
)

const Signature = "jpegli-encode.go"

func main() {
	input := flag.String("input", "", "Source file (required)")
	output := flag.String("output", "", "Destination file (optional)")
	quality := flag.Int("quality", 90, "Quality (1-100, default 90)")
	chroma := flag.String("chroma_subsampling", "444", "Chroma subsampling: 444, 422, 420")
	
	flag.Parse()

	if *input == "" {
		if len(os.Args) == 1 {
			flag.Usage()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: -input is required\n")
		flag.Usage()
		os.Exit(1)
	}

	absSrc, err := filepath.Abs(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving input path: %v\n", err)
		os.Exit(1)
	}

	srcInfo, err := os.Stat(absSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error stating input file: %v\n", err)
		os.Exit(1)
	}

	finalDest := *output
	if finalDest == "" {
		finalDest = *input
	}

	// Read source data
	srcData, err := os.ReadFile(absSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading source: %v\n", err)
		os.Exit(1)
	}

	// Decode image
	img, _, err := image.Decode(bytes.NewReader(srcData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding image: %v\n", err)
		os.Exit(1)
	}

	// Map chroma subsampling
	var ratio image.YCbCrSubsampleRatio
	switch *chroma {
	case "444":
		ratio = image.YCbCrSubsampleRatio444
	case "422":
		ratio = image.YCbCrSubsampleRatio422
	case "420":
		ratio = image.YCbCrSubsampleRatio420
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid chroma subsampling '%s' (use 444, 422, or 420)\n", *chroma)
		os.Exit(1)
	}

	// Encode with jpegli
	var buf bytes.Buffer
	err = jpegli.Encode(&buf, img, &jpegli.EncodingOptions{
		Quality:           *quality,
		ChromaSubsampling: ratio,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding with jpegli: %v\n", err)
		os.Exit(1)
	}

	tempPath := absSrc + ".tmp_jpegli"
	if err := os.WriteFile(tempPath, buf.Bytes(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing temp file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(tempPath)

	// Apply metadata (preserves everything except large segments, same logic as main.go default)
	if err := copyJPEGMetadata(absSrc, tempPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Error applying metadata: %v\n", err)
	}

	// Prepare destination directory if needed
	if *output != "" {
		if err := os.MkdirAll(filepath.Dir(finalDest), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Handle overwrite or move
	if finalDest == absSrc {
		if err := os.Remove(absSrc); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting source: %v\n", err)
			os.Exit(1)
		}
	}

	if err := moveFile(tempPath, finalDest); err != nil {
		fmt.Fprintf(os.Stderr, "Error moving final file: %v\n", err)
		os.Exit(1)
	}

	// Restore time and permissions
	_ = os.Chtimes(finalDest, srcInfo.ModTime(), srcInfo.ModTime())
	_ = os.Chmod(finalDest, srcInfo.Mode())

	finalInfo, _ := os.Stat(finalDest)
	sizeBefore := srcInfo.Size()
	sizeAfter := finalInfo.Size()
	gain := 100 - (float64(sizeAfter) / float64(sizeBefore) * 100)

	fmt.Printf("Successfully encoded %s to %s (quality %d)\n", *input, finalDest, *quality)
	fmt.Printf("Size: %s -> %s (Gain: %.1f%%)\n", formatSize(sizeBefore), formatSize(sizeAfter), gain)
}

func formatSize(size int64) string {
	if size >= 1048576 {
		return fmt.Sprintf("%.2f MB", float64(size)/1048576)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
}

func moveFile(src, dst string) error {
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	
	// Fallback for cross-device
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil { return err }
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	
	return os.Remove(src)
}

func copyJPEGMetadata(src, dst string) error {
	srcData, err := os.ReadFile(src)
	if err != nil { return err }
	dstData, err := os.ReadFile(dst)
	if err != nil { return err }

	var segments [][]byte
	// Extract APPn segments from source
	for i := 0; i < len(srcData)-1; {
		if srcData[i] != 0xFF { i++; continue }
		marker := srcData[i+1]
		if marker == 0x00 || marker == 0xFF { i++; continue }
		if marker == 0xD8 { i += 2; continue }
		if marker == 0xDA || marker == 0xC0 || marker == 0xC2 { break }
		
		if i+3 >= len(srcData) { break }
		length := int(srcData[i+2])<<8 | int(srcData[i+3])
		
		if marker >= 0xE0 && marker <= 0xEF {
			if i+2+length <= len(srcData) {
				segment := srcData[i : i+2+length]
				keep := true
				
				// Filtering logic (strip large segments like in main.go)
				// 1. Strip Extended XMP
				if marker == 0xE1 && length > 35 {
					header := string(segment[4:33])
					if header == "http://ns.adobe.com/xmp/exten" {
						keep = false
					}
				}
				// 2. Strip Photoshop thumbnails/binary data (APP13)
				if marker == 0xED && length > 14 {
					header := string(segment[4:14])
					if header == "Photoshop " {
						keep = false
					}
				}
				// 3. Strip FPXR (FlashPix)
				if marker == 0xE2 && length > 10 {
					header := string(segment[4:9])
					if header == "FPXR" {
						keep = false
					}
				}

				if keep {
					segments = append(segments, segment)
				}
			}
		}
		i += 2 + length
	}

	// Create new JPEG
	var out bytes.Buffer
	out.Write([]byte{0xFF, 0xD8}) // SOI
	
	// Signature
	sigData := []byte(Signature)
	out.Write([]byte{0xFF, 0xEF}) // APP15 marker
	out.Write([]byte{byte((len(sigData)+2) >> 8), byte((len(sigData)+2) & 0xFF)})
	out.Write(sigData)

	for _, seg := range segments {
		out.Write(seg)
	}
	
	// Find start of image data in destination
	imgDataIndex := -1
	for i := 0; i < len(dstData)-1; {
		if dstData[i] == 0xFF {
			marker := dstData[i+1]
			if marker == 0x00 || marker == 0xFF { i++; continue }
			if marker == 0xD8 { i += 2; continue }
			if (marker < 0xE0 || marker > 0xEF) && marker != 0xFE {
				imgDataIndex = i
				break
			}
			if i+3 >= len(dstData) { break }
			length := int(dstData[i+2])<<8 | int(dstData[i+3])
			i += 2 + length
		} else {
			i++
		}
	}

	if imgDataIndex != -1 {
		out.Write(dstData[imgDataIndex:])
	} else if len(dstData) > 2 {
		out.Write(dstData[2:]) // Fallback
	}

	return os.WriteFile(dst, out.Bytes(), 0644)
}

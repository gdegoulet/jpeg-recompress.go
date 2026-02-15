package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gen2brain/jpegli"
	"github.com/jasonmoo/go-butteraugli"
	"golang.org/x/image/draw"
)

const Signature = "jpeg-recompress.go"

var Version = "dev"

type Result struct {
	SizeBefore int64
	SizeAfter  int64
	BestQ      int
	Skipped    bool
	Copied     bool
	MSE        float64
	SSIM       float64
	PSNR       float64
	Duration   time.Duration
	Err        error
}

type VerificationResults struct {
	IsSmallerOrEqual bool `json:"is_smaller_or_equal"`
	SamePermissions  bool `json:"same_permissions"`
	SameModTime      bool `json:"same_mod_time"`
}

type FinalOutput struct {
	Status        string  `json:"status"`
	Input         string  `json:"input"`
	Output        string  `json:"output"`
	SizeBefore    int64   `json:"size_before_bytes"`
	SizeAfter     int64   `json:"size_after_bytes"`
	GainPercent   float64 `json:"gain_percent"`
	Quality       int     `json:"best_q"`
	Metric        string  `json:"metric_used"`
	Threshold     float64 `json:"threshold"`
	Sample        int     `json:"sample"`
	MSE           float64 `json:"mse"`
	SSIM          float64 `json:"ssim"`
	PSNR          float64 `json:"psnr_db"`
	ExecutionTime string  `json:"execution_time"`
	Test          VerificationResults `json:"test_results"`
}

func main() {
	input := flag.String("input", "", "Source file (required)")
	output := flag.String("output", "", "Destination file (optional)")
	metric := flag.String("metric", "psnr", "Metric: psnr, ssim, mse or butteraugli")
	targetQuality := flag.Float64("threshold", -1.0, "Threshold (Default: PSNR=38.5, SSIM=0.99, MSE=0.99995)")
	sample := flag.Int("sample", 0, "Sub-sampling (0=auto)")
	minQ := flag.Int("min-quality", 70, "Minimum quality (default 70)")
	maxQ := flag.Int("max-quality", 90, "Maximum quality (default 90)")
	keepAll := flag.Bool("keep-all-metadata", false, "Keep all metadata")
	skipMeta := flag.Bool("skip-metadata", false, "Strip all metadata")
	quiet := flag.Bool("quiet", false, "Quiet mode")
	debug := flag.Bool("debug", false, "Debug mode")
	fast := flag.Bool("fast", false, "Fast mode")
	version := flag.Bool("version", false, "Show version")
	useJpegli := flag.Bool("jpegli", false, "Use Jpegli encoder (experimental)")

	flag.Parse()

	if *useJpegli {
		*metric = "butteraugli"
	}

	if *version {
		fmt.Printf("jpeg-recompress.go version %s\n", Version)
		os.Exit(0)
	}
	var local_startTime time.Time
	var duration time.Duration

	if len(os.Args) < 2 {
		flag.CommandLine.SetOutput(os.Stderr)
		flag.Usage()
		os.Exit(0)
	}

	if *input == "" {
		fmt.Fprintf(os.Stderr, `{"error": "The -input option is required"}`+"\n")
		os.Exit(1)
	}

	if *targetQuality == -1.0 {
		switch strings.ToLower(*metric) {
		case "ssim":
			*targetQuality = 0.99
		case "mse":
			*targetQuality = 0.99995
		case "butteraugli":
			*targetQuality = 1.0
		default: // psnr
			*targetQuality = 38.5
		}
	}

	local_startTime = time.Now()
	if err := checkDependencies(); err != nil {
		fmt.Fprintf(os.Stderr, `{"error": "Missing dependencies", "details": "%v"}`+"\n", err)
		os.Exit(1)
	}
	duration = time.Since(local_startTime)
	if *debug { fmt.Fprintf(os.Stderr, "[DEBUG] checkDependencies duration=%s\n", duration.Round(time.Millisecond).String()) }

	finalDest := *output
	if finalDest == "" { finalDest = *input }

	if *debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Computing %s\n", *input)
	}
	res, actualSample, srcFileInfo, finalFileInfo := processSingleFile(*input, *output, *targetQuality, *minQ, *maxQ, *keepAll, *skipMeta, *metric, *sample, *debug, *fast, *useJpegli)

	status := "SUCCESS"
	if res.Skipped {
		status = "SKIPPED"
	} else if res.Copied {
		status = "COPIED_NO_GAIN"
	}

			gain := 0.0
			if res.SizeBefore > 0 {
				gain = 100 - (float64(res.SizeAfter) / float64(res.SizeBefore) * 100)
			}
	
			verification := VerificationResults{}
			if srcFileInfo != nil && finalFileInfo != nil {
				verification.IsSmallerOrEqual = res.SizeAfter <= srcFileInfo.Size()
				verification.SamePermissions = finalFileInfo.Mode() == srcFileInfo.Mode()
				verification.SameModTime = finalFileInfo.ModTime().Equal(srcFileInfo.ModTime())
			}
	
			isPerfect := status == "SUCCESS" && verification.IsSmallerOrEqual && verification.SamePermissions && verification.SameModTime
	
			// Exit code determination based on business rules
			shouldExitZero := isPerfect
			if !isPerfect && res.Err == nil {
				if *output == "" && status == "SKIPPED" {
					shouldExitZero = true
				} else if *output != "" && status == "COPIED_NO_GAIN" {
					shouldExitZero = true
				}
			}

			if !*quiet {
				if res.Err != nil {
					fmt.Fprintf(os.Stderr, `{"error": "Processing failed", "file": "%s", "details": "%v"}`+"\n", *input, res.Err)
					os.Exit(1)
				}
		out := FinalOutput{
			Status: status, Input: *input, Output: finalDest,
			GainPercent: math.Round(gain*10) / 10, Quality: res.BestQ,
			SizeBefore: res.SizeBefore, SizeAfter: res.SizeAfter,
			Metric: strings.ToUpper(*metric), Threshold: *targetQuality, Sample: actualSample,
			MSE: res.MSE, SSIM: res.SSIM, PSNR: math.Round(res.PSNR*10) / 10,
			ExecutionTime: res.Duration.Round(time.Millisecond).String(),
			Test:          verification,
		}
		jsonBytes, _ := json.Marshal(out)
		fmt.Println(string(jsonBytes))
	}

	if !shouldExitZero || res.Err != nil {
		os.Exit(1)
	}
}


func getAdaptiveSample(b image.Rectangle, debug bool) int {
	pixels := b.Dx() * b.Dy()
	if pixels > 128000000 { return 0 } // Limite à 128MP
	switch {
	case pixels <= 1000000: return 1   // < 1MP
	case pixels <= 4000000: return 2   // < 4MP
	case pixels <= 16000000: return 4  // < 16MP
	case pixels <= 64000000: return 8  // < 64MP
	default: return 16                 // > 64MP
	}
}

// mapStdToJpegli converts a standard JPEG quality to a visually equivalent Jpegli quality
func mapStdToJpegli(stdQ int) int {
	switch {
	case stdQ >= 95: return stdQ - 2
	case stdQ >= 90: return stdQ - 4
	case stdQ >= 85: return stdQ - 6
	case stdQ >= 80: return stdQ - 8
	case stdQ >= 75: return stdQ - 10
	default: return stdQ - 12
	}
}

func processSingleFile(src, dst string, threshold float64, minQ, maxQ int, keepAll, skipMeta bool, metric string, sample int, debug, fast, useJpegli bool) (Result, int, os.FileInfo, os.FileInfo) {
	startTime := time.Now()
	res := Result{}
	absSrc, _ := filepath.Abs(src)
	srcInfo, err := os.Stat(absSrc)
	if err != nil { res.Err = err; return res, 0, srcInfo, nil }
	res.SizeBefore = srcInfo.Size()
	originalModTime := srcInfo.ModTime()

	if isAlreadyProcessed(absSrc) {
		if dst != "" {
			_ = os.MkdirAll(filepath.Dir(dst), 0755)
			_ = copyFile(absSrc, dst)
			_ = os.Chtimes(dst, originalModTime, originalModTime)
			res.Copied = true
		} else {
			res.Skipped = true
		}
		res.SizeAfter = res.SizeBefore
		res.Duration = time.Since(startTime)
		var fInfo os.FileInfo
		if res.Copied {
			fInfo, _ = os.Stat(dst)
		} else if dst == "" {
			fInfo, _ = os.Stat(absSrc)
		}
		return res, 0, srcInfo, fInfo
	}

	srcData, _ := os.ReadFile(absSrc)
	img, _, err := image.Decode(bytes.NewReader(srcData))
	if err != nil { res.Err = err; return res, 0, srcInfo, nil }

	actualSample := sample
	if actualSample <= 0 { actualSample = getAdaptiveSample(img.Bounds(), debug) }
	if actualSample == 0 {
		res.Skipped = true; res.SizeAfter = res.SizeBefore
		res.Duration = time.Since(startTime)
		var fInfo os.FileInfo
		if dst == "" { fInfo, _ = os.Stat(absSrc) }
		return res, 0, srcInfo, fInfo
	}

	var local_startTime time.Time 
	var duration time.Duration 
	var bestData []byte
	bestQ := minQ
	lowQ, highQ := minQ, maxQ
	step := 1
	if fast { step = 2 }

	// Search phase: we ALWAYS use the standard encoder for speed
	// and as a baseline for the metric threshold.
	for lowQ <= highQ {
		currentQ := (lowQ + highQ) / 2
		if step > 1 { currentQ = (currentQ / step) * step }

		local_startTime = time.Now()
		var buf bytes.Buffer
		_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: currentQ})
		
		duration = time.Since(local_startTime)
		if debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] currentQ=%d Encode to std-jpg duration=%s\n", currentQ, duration.Round(time.Millisecond).String()) 
		}

		local_startTime = time.Now()
		compImg, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
		duration = time.Since(local_startTime)
		if err != nil || compImg == nil { lowQ = currentQ + step; continue }

		var sim float64
		switch strings.ToLower(metric) {
		case "ssim":
			sim = calculateSSIM(img, compImg, actualSample)
		case "mse":
			sim = 1.0 - calculateMSE(img, compImg, actualSample)
		case "butteraugli":
			sim = calculateButteraugli(img, compImg)
		default:
			sim = calculatePSNR(img, compImg, actualSample)
		}

		// Butteraugli: smaller is better, others: larger is better
		isBetter := sim >= threshold
		if strings.ToLower(metric) == "butteraugli" {
			isBetter = sim <= threshold
		}

		if !isBetter {
			lowQ = currentQ + step
		} else {
			bestQ = currentQ; highQ = currentQ - step; bestData = buf.Bytes()
		}
	}

	targetPath := dst
	if targetPath == "" { targetPath = absSrc }

	// Size gain check: if no improvement, skip or copy original
	if bestData == nil || int64(len(bestData)) >= res.SizeBefore {
		if dst != "" {
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			_ = copyFile(absSrc, targetPath)
			_ = os.Chtimes(targetPath, originalModTime, originalModTime)
			res.Copied = true
			res.SizeAfter = res.SizeBefore
		} else {
			res.Skipped = true
			res.SizeAfter = res.SizeBefore
		}

		if res.Skipped || res.Copied {
			res.Duration = time.Since(startTime)
			var fInfo os.FileInfo
			if res.Copied { fInfo, _ = os.Stat(targetPath) } else if dst == "" { fInfo, _ = os.Stat(absSrc) }
			return res, actualSample, srcInfo, fInfo
		}
	}

	// Final rendering phase: if Jpegli is requested, we map the quality
	if useJpegli && bestData != nil {
		liQ := mapStdToJpegli(bestQ)
		if debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] Calibrating Jpegli: StdQ %d -> JpegliQ %d\n", bestQ, liQ)
		}
		var buf bytes.Buffer
		local_startTime = time.Now()
		_ = jpegli.Encode(&buf, img, &jpegli.EncodingOptions{
			Quality:           liQ,
			ChromaSubsampling: image.YCbCrSubsampleRatio420,
		})
		duration = time.Since(local_startTime)
		if debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] Jpegli Final Encode duration=%s\n", duration.Round(time.Millisecond).String())
		}
		bestData = buf.Bytes()
		bestQ = liQ // Update for final JSON output
	}

	finalImg, _, _ := image.Decode(bytes.NewReader(bestData))
	if finalImg == nil {
		res.Skipped = true
		res.SizeAfter = res.SizeBefore
		fInfo, _ := os.Stat(targetPath)
		return res, actualSample, srcInfo, fInfo
	}
	
	res.MSE = calculateMSE(img, finalImg, actualSample)
	res.SSIM = calculateSSIM(img, finalImg, actualSample)
	res.PSNR = calculatePSNR(img, finalImg, actualSample)
	res.BestQ = bestQ

	tempPath := absSrc + ".tmp_recompress"
	if err := os.WriteFile(tempPath, bestData, 0644); err != nil {
		res.Err = fmt.Errorf("error writing temp file: %v", err)
		return res, actualSample, srcInfo, nil
	}
	defer os.Remove(tempPath)

	applyMetadata(absSrc, tempPath, keepAll, skipMeta)

	if dst != "" {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			res.Err = fmt.Errorf("error creating directory: %v", err)
			return res, actualSample, srcInfo, nil
		}
	}
	if targetPath == absSrc {
		if err := os.Remove(absSrc); err != nil {
			res.Err = fmt.Errorf("error deleting source: %v", err)
			return res, actualSample, srcInfo, nil
		}
	}
	if err := moveFile(tempPath, targetPath); err != nil {
		res.Err = fmt.Errorf("error moving final file: %v", err)
		return res, actualSample, srcInfo, nil
	}
	
	_ = os.Chtimes(targetPath, originalModTime, originalModTime)
	_ = os.Chmod(targetPath, srcInfo.Mode())

	finalInfo, _ := os.Stat(targetPath)
	res.SizeAfter = finalInfo.Size()
	res.Duration = time.Since(startTime)
	return res, actualSample, srcInfo, finalInfo
}


func calculatePSNR(img1, img2 image.Image, sample int) float64 {
	b := img1.Bounds()
	var sum, count float64
	for y := b.Min.Y; y < b.Max.Y; y += sample {
		for x := b.Min.X; x < b.Max.X; x += sample {
			r1, g1, b1, _ := img1.At(x, y).RGBA()
			r2, g2, b2, _ := img2.At(x, y).RGBA()
			dr, dg, db := float64(r1>>8)-float64(r2>>8), float64(g1>>8)-float64(g2>>8), float64(b1>>8)-float64(b2>>8)
			sum += (dr*dr + dg*dg + db*db) / 3.0
			count++
		}
	}
	mse := sum / count
	if mse == 0 { return 100.0 }
	return 20*math.Log10(255) - 10*math.Log10(mse)
}

func calculateMSE(img1, img2 image.Image, sample int) float64 {
	b := img1.Bounds()
	var sum, count float64
	for y := b.Min.Y; y < b.Max.Y; y += sample {
		for x := b.Min.X; x < b.Max.X; x += sample {
			r1, g1, b1, _ := img1.At(x, y).RGBA()
			r2, g2, b2, _ := img2.At(x, y).RGBA()
			dr, dg, db := float64(r1>>8)-float64(r2>>8), float64(g1>>8)-float64(g2>>8), float64(b1>>8)-float64(b2>>8)
			sum += (dr*dr + dg*dg + db*db) / (3.0 * 255 * 255)
			count++
		}
	}
	return sum / count
}

func calculateSSIM(img1, img2 image.Image, sample int) float64 {
	b := img1.Bounds()
	w, h := b.Dx(), b.Dy()
	const (c1, c2 = 6.5025, 58.5225)
	var total, count float64
	step := 8 * sample
	for y := 0; y < h; y += step {
		for x := 0; x < w; x += step {
			var m1, m2, s1, s2, s12, n float64
			for by := y; by < y+8 && by < h; by++ {
				for bx := x; bx < x+8 && bx < w; bx++ {
					v1, v2 := getLuminance(img1.At(bx, by)), getLuminance(img2.At(bx, by))
					m1 += v1; m2 += v2; n++
				}
			}
			m1 /= n; m2 /= n
			for by := y; by < y+8 && by < h; by++ {
				for bx := x; bx < x+8 && bx < w; bx++ {
					v1, v2 := getLuminance(img1.At(bx, by)), getLuminance(img2.At(bx, by))
					s1 += (v1 - m1) * (v1 - m1); s2 += (v2 - m2) * (v2 - m2); s12 += (v1 - m1) * (v2 - m2)
				}
			}
			if n > 1 {
				s1 /= (n - 1); s2 /= (n - 1); s12 /= (n - 1)
			} else {
				s1, s2, s12 = 0, 0, 0
			}
			total += ((2*m1*m2 + c1) * (2*s12 + c2)) / ((m1*m1 + m2*m2 + c1) * (s1 + s2 + c2))
			count++
		}
	}
	return total / count
}

func getLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)
}

func calculateButteraugli(img1, img2 image.Image) float64 {
	// Optimization: Butteraugli is extremely slow on large images.
	// We downsample to a maximum of 0.5 Megapixels for analysis.
	// This preserves perceptual patterns while being ~10-20x faster.
	const maxPixels = 500000
	b := img1.Bounds()
	origPixels := b.Dx() * b.Dy()

	if origPixels <= maxPixels {
		dist, _ := butteraugli.CompareImages(img1, img2)
		return dist
	}

	// Calculate scaling factor
	scale := math.Sqrt(float64(maxPixels) / float64(origPixels))
	newW, newH := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
	newRect := image.Rect(0, 0, newW, newH)

	// Create downsampled images
	small1 := image.NewRGBA(newRect)
	small2 := image.NewRGBA(newRect)

	// BiLinear is a good balance between speed and quality for perceptual analysis
	draw.BiLinear.Scale(small1, newRect, img1, b, draw.Over, nil)
	draw.BiLinear.Scale(small2, newRect, img2, b, draw.Over, nil)

	dist, _ := butteraugli.CompareImages(small1, small2)
	return dist
}

func applyMetadata(src, dst string, keepAll, skipMeta bool) {
	ext := strings.ToLower(filepath.Ext(src))
	if ext == ".jpg" || ext == ".jpeg" {
		_ = copyJPEGMetadata(src, dst, keepAll, skipMeta)
	}
}

func isAlreadyProcessed(src string) bool {
	data, err := os.ReadFile(src)
	if err != nil { return false }
	
	// Look at the beginning (JPEG COM)
	limit := 32768
	if len(data) < limit { limit = len(data) }
	if strings.Contains(string(data[:limit]), Signature) {
		return true
	}

	return false
}


func checkDependencies() error {
	// No external dependencies required (all native Go for JPEG)
	return nil
}

func countMetadata(path string) int {
	data, err := os.ReadFile(path)
	if err != nil { return 0 }
	count := 0
	
	// JPEG version: count APP or COM segments properly
	for i := 0; i < len(data)-1; {
		if data[i] == 0xFF {
			marker := data[i+1]
			if marker == 0x00 || marker == 0xFF {
				i++
				continue
			}
			if marker == 0xD8 { // SOI
				i += 2
				continue
			}
			if marker == 0xDA || marker == 0xC0 || marker == 0xC2 { // SOS or SOF
				break
			}
			if i+3 >= len(data) { break }
			length := int(data[i+2])<<8 | int(data[i+3])
			
			if marker >= 0xE0 && marker <= 0xFE {
				count++
			}
			i += 2 + length
		} else {
			i++
		}
	}
	return count
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()

	srcInfo, err := in.Stat()
	if err != nil { return err }

	out, err := os.Create(dst)
	if err != nil { return err }
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil { return err }

	return os.Chmod(dst, srcInfo.Mode())
}

func moveFile(src, dst string) error {
	// Tente un renommage atomique (rapide, même partition)
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Si erreur de lien cross-device, on copie puis supprime
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyJPEGMetadata(src, dst string, keepAll, skipMeta bool) error {
	srcData, err := os.ReadFile(src)
	if err != nil { return err }
	dstData, err := os.ReadFile(dst)
	if err != nil { return err }

	var segments [][]byte
	if !skipMeta {
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
					
					// Filtering logic for Default mode (keepAll=false)
					if !keepAll {
						// 1. Strip Extended XMP (often used for heavy payloads like depth maps or videos)
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
						// 3. Strip FPXR (FlashPix) which is usually large and useless
						if marker == 0xE2 && length > 10 {
							header := string(segment[4:9])
							if header == "FPXR" {
								keep = false
							}
						}
					}

					if keep {
						segments = append(segments, segment)
					}
				}
			}
			i += 2 + length
		}
	}

	// Create new JPEG
	var out bytes.Buffer
	out.Write([]byte{0xFF, 0xD8}) // SOI
	
	// Signature injection (APP15 segment) - EARLY in file
	sigData := []byte(Signature)
	out.Write([]byte{0xFF, 0xEF}) // APP15 marker
	out.Write([]byte{byte((len(sigData)+2) >> 8), byte((len(sigData)+2) & 0xFF)})
	out.Write(sigData)

	for _, seg := range segments {
		// Avoid duplicating our own signature if it was already in an APP15
		if len(seg) > 2 && seg[1] == 0xEF && strings.Contains(string(seg), Signature) {
			continue
		}
		out.Write(seg)
	}
	
	// Find start of image data in destination (DQT, DHT, SOF, SOS etc.)
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

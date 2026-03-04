package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
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
	stdDraw "golang.org/x/image/draw"
)

const Signature = "jpeg-recompress.go"

var Version = "dev"

type Result struct {
	SizeBefore  int64
	SizeAfter   int64
	BestQ       int
	Skipped     bool
	Copied      bool
	MSE         float64
	SSIM        float64
	PSNR        float64
	Butteraugli float64
	Duration    time.Duration
	Err         error
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
	Butteraugli   float64 `json:"butteraugli_score"`
	ExecutionTime string  `json:"execution_time"`
	Test          VerificationResults `json:"test_results"`
}

func main() {
	input := flag.String("input", "", "Source file (required)")
	output := flag.String("output", "", "Destination file (optional)")
	metric := flag.String("metric", "psnr", "Metric: psnr, ssim, mse or butteraugli")
	targetQuality := flag.Float64("threshold", -1.0, "Threshold (Default: PSNR=39.5, SSIM=0.99, MSE=0.99995)")
	sample := flag.Int("sample", 0, "Sub-sampling (0=auto)")
	minQ := flag.Int("min-quality", 70, "Minimum quality (default 70)")
	maxQ := flag.Int("max-quality", 90, "Maximum quality (default 90)")
	chroma := flag.String("chroma_subsampling", "444", "Chroma subsampling: 444, 422, 420 (for Jpegli)")
	keepAll := flag.Bool("keep-all-metadata", false, "Keep all metadata")
	skipMeta := flag.Bool("skip-metadata", false, "Strip all metadata")
	quiet := flag.Bool("quiet", false, "Quiet mode")
	debug := flag.Bool("debug", false, "Debug mode")
	fast := flag.Bool("fast", false, "Fast mode")
	allMetrics := flag.Bool("all-metrics", false, "Compute all metrics including butteraugli (slower)")
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
	if len(os.Args) < 2 {
		flag.CommandLine.SetOutput(os.Stderr)
		flag.Usage()
		os.Exit(0)
	}

	if *input == "" {
		fmt.Fprintf(os.Stderr, `{"error": "The -input option is required"}`+"\n")
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
		fmt.Fprintf(os.Stderr, `{"error": "Invalid chroma subsampling '%s' (use 444, 422, or 420)"}`+"\n", *chroma)
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
			*targetQuality = 39.5
		}
	}

	finalDest := *output
	if finalDest == "" { finalDest = *input }

	if *debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Computing %s\n", *input)
	}
	res, actualSample, srcFileInfo, finalFileInfo := processSingleFile(*input, *output, *targetQuality, *minQ, *maxQ, ratio, *keepAll, *skipMeta, *metric, *sample, *debug, *fast, *useJpegli, *allMetrics)

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
		// We consider it a "soft success" if we didn't gain anything but handled it safely
		if status == "SKIPPED" || status == "COPIED_NO_GAIN" {
			shouldExitZero = true
		}
	}

	if res.Err != nil {
		fmt.Fprintf(os.Stderr, `{"error": "Processing failed", "file": "%s", "details": "%v"}`+"\n", *input, res.Err)
		os.Exit(1)
	}

	if !*quiet {
		out := FinalOutput{
			Status: status, Input: *input, Output: finalDest,
			GainPercent: math.Round(gain*10) / 10, Quality: res.BestQ,
			SizeBefore: res.SizeBefore, SizeAfter: res.SizeAfter,
			Metric: strings.ToUpper(*metric), Threshold: *targetQuality, Sample: actualSample,
			MSE: res.MSE, SSIM: res.SSIM, PSNR: math.Round(res.PSNR*10) / 10,
			Butteraugli:   math.Round(res.Butteraugli*1000) / 1000,
			ExecutionTime: res.Duration.Round(time.Millisecond).String(),
			Test:          verification,
		}
		jsonBytes, _ := json.Marshal(out)
		fmt.Println(string(jsonBytes))
	}

	if !shouldExitZero {
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

func processSingleFile(src, dst string, threshold float64, minQ, maxQ int, ratio image.YCbCrSubsampleRatio, keepAll, skipMeta bool, metric string, sample int, debug, fast, useJpegli, allMetrics bool) (Result, int, os.FileInfo, os.FileInfo) {
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

	srcData, err := os.ReadFile(absSrc)
	if err != nil { res.Err = err; return res, 0, srcInfo, nil }
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

	var encodeStart time.Time
	var encodeDuration time.Duration
	var bestData []byte
	var bestImg image.Image // cached decoded image from winning search iteration
	bestQ := minQ
	lowQ, highQ := minQ, maxQ
	step := 1
	if fast { step = 2 }

	// Search phase
	for lowQ <= highQ {
		currentQ := (lowQ + highQ) / 2
		if step > 1 { currentQ = (currentQ / step) * step }

		encodeStart = time.Now()
		var buf bytes.Buffer
		
		if useJpegli {
			_ = jpegli.Encode(&buf, img, &jpegli.EncodingOptions{
				Quality:           currentQ,
				ChromaSubsampling: ratio,
			})
		} else {
			_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: currentQ})
		}
		
		encodeDuration = time.Since(encodeStart)

		decodeStart := time.Now()
		compImg, _, err := image.Decode(bytes.NewReader(buf.Bytes()))
		durationDecode := time.Since(decodeStart)
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

		if debug {
			encoderName := "std-jpg"
			if useJpegli { encoderName = "jpegli" }
			currentSize := int64(buf.Len())
			gain := 100 - (float64(currentSize) / float64(res.SizeBefore) * 100)
			
			fmt.Fprintf(os.Stderr, "[DEBUG] currentQ=%d Encode to %s duration=%s Metric=%s Score=%.4f (Threshold=%.2f) Size=%s Gain=%.1f%%\n", 
				currentQ, encoderName, encodeDuration.Round(time.Millisecond).String(), 
				strings.ToUpper(metric), sim, threshold, formatSize(currentSize), gain)
			if debug && durationDecode > 50*time.Millisecond {
				// Only log decode if significant
				fmt.Fprintf(os.Stderr, "[DEBUG]   (Decode took %s)\n", durationDecode.Round(time.Millisecond).String())
			}
		}

		// Butteraugli: smaller is better, others: larger is better
		isBetter := sim >= threshold
		if strings.ToLower(metric) == "butteraugli" {
			isBetter = sim <= threshold
		}

		if isBetter {
			// Current quality meets threshold, try even lower quality to save more space
			bestQ = currentQ
			highQ = currentQ - step
			bestData = buf.Bytes()
			bestImg = compImg
		} else {
			// Current quality does NOT meet threshold, must increase quality
			lowQ = currentQ + step
		}
	}

	targetPath := dst
	if targetPath == "" {
		targetPath = absSrc
	} else {
		targetPath, _ = filepath.Abs(targetPath)
	}

	// If no quality in the search range met the threshold, fall back to no-gain path
	if len(bestData) == 0 {
		if targetPath == absSrc {
			res.Skipped = true
			res.SizeAfter = srcInfo.Size()
			res.Duration = time.Since(startTime)
			return res, actualSample, srcInfo, srcInfo
		}
		_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
		if err := copyFile(absSrc, targetPath); err != nil {
			res.Err = fmt.Errorf("error copying original to destination: %v", err)
			return res, actualSample, srcInfo, nil
		}
		_ = os.Chtimes(targetPath, originalModTime, originalModTime)
		res.Copied = true
		res.SizeAfter = srcInfo.Size()
		res.Duration = time.Since(startTime)
		finalInfo, _ := os.Stat(targetPath)
		return res, actualSample, srcInfo, finalInfo
	}

	// Use cached decoded image from search loop; fallback to re-decode only if needed
	finalImg := bestImg
	if finalImg == nil {
		finalImg, _, _ = image.Decode(bytes.NewReader(bestData))
	}
	if finalImg != nil {
		res.MSE, res.PSNR = calculateMSEAndPSNR(img, finalImg, actualSample)
		res.SSIM = calculateSSIM(img, finalImg, actualSample)
		// Butteraugli is expensive; only compute it when it was the active metric or explicitly requested
		if strings.ToLower(metric) == "butteraugli" || allMetrics {
			res.Butteraugli = calculateButteraugli(img, finalImg)
		}
	}
	res.BestQ = bestQ

	tempPath := absSrc + ".tmp_recompress"
	if err := os.WriteFile(tempPath, bestData, 0644); err != nil {
		res.Err = fmt.Errorf("error writing temp file: %v", err)
		return res, actualSample, srcInfo, nil
	}
	defer os.Remove(tempPath)

	if err := applyMetadata(absSrc, tempPath, keepAll, skipMeta); err != nil {
		res.Err = fmt.Errorf("error applying metadata: %v", err)
		return res, actualSample, srcInfo, nil
	}

	tempInfo, _ := os.Stat(tempPath)

	// Check if we actually gained something
	if tempInfo.Size() >= srcInfo.Size() {
		if debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] No gain (new: %s, old: %s).\n", formatSize(tempInfo.Size()), formatSize(srcInfo.Size()))
		}
		if targetPath == absSrc {
			res.Skipped = true
			res.SizeAfter = srcInfo.Size()
			res.Duration = time.Since(startTime)
			return res, actualSample, srcInfo, srcInfo
		} else {
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			if err := copyFile(absSrc, targetPath); err != nil {
				res.Err = fmt.Errorf("error copying original to destination: %v", err)
				return res, actualSample, srcInfo, nil
			}
			_ = os.Chtimes(targetPath, originalModTime, originalModTime)
			res.Copied = true
			res.SizeAfter = srcInfo.Size()
			res.Duration = time.Since(startTime)
			finalInfo, _ := os.Stat(targetPath)
			return res, actualSample, srcInfo, finalInfo
		}
	}

	// Prepare destination directory if needed
	if dst != "" {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			res.Err = fmt.Errorf("error creating directory: %v", err)
			return res, actualSample, srcInfo, nil
		}
	}

	// Critical section: atomic-like move or copy
	if targetPath == absSrc {
		// Overwrite mode: We have the original in absSrc and the new one in tempPath
		// We move tempPath to a SECOND temp name, then rename to source, to be as safe as possible
		// But os.Rename is already atomic on most systems.
		// The safest way is to rename tempPath to absSrc directly.
		if err := moveFile(tempPath, targetPath); err != nil {
			res.Err = fmt.Errorf("error overwriting source file: %v", err)
			return res, actualSample, srcInfo, nil
		}
	} else {
		// Output to different file
		if err := moveFile(tempPath, targetPath); err != nil {
			res.Err = fmt.Errorf("error moving to destination: %v", err)
			return res, actualSample, srcInfo, nil
		}
	}
	
	_ = os.Chtimes(targetPath, originalModTime, originalModTime)
	_ = os.Chmod(targetPath, srcInfo.Mode())

	finalInfo, _ := os.Stat(targetPath)
	res.SizeAfter = finalInfo.Size()
	res.Duration = time.Since(startTime)
	return res, actualSample, srcInfo, finalInfo
}


// toNRGBA converts any image.Image to *image.NRGBA for direct pixel access.
// Only use this when the image is small or when it will be accessed many times.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, img.At(x, y))
		}
	}
	return dst
}

// calculateMSEAndPSNR computes both MSE and PSNR in a single pixel pass.
func calculateMSEAndPSNR(img1, img2 image.Image, sample int) (mse, psnr float64) {
	b := img1.Bounds()
	var sum, count float64
	for y := b.Min.Y; y < b.Max.Y; y += sample {
		for x := b.Min.X; x < b.Max.X; x += sample {
			r1, g1, b1, _ := img1.At(x, y).RGBA()
			r2, g2, b2, _ := img2.At(x, y).RGBA()
			dr := float64(r1>>8) - float64(r2>>8)
			dg := float64(g1>>8) - float64(g2>>8)
			db := float64(b1>>8) - float64(b2>>8)
			sum += (dr*dr + dg*dg + db*db) / 3.0
			count++
		}
	}
	rawMSE := sum / count
	mse = rawMSE / (255 * 255)
	if rawMSE == 0 {
		psnr = 100.0
	} else {
		psnr = 20*math.Log10(255) - 10*math.Log10(rawMSE)
	}
	return
}

func calculatePSNR(img1, img2 image.Image, sample int) float64 {
	_, psnr := calculateMSEAndPSNR(img1, img2, sample)
	return psnr
}

func calculateMSE(img1, img2 image.Image, sample int) float64 {
	mse, _ := calculateMSEAndPSNR(img1, img2, sample)
	return mse
}

func calculateSSIM(img1, img2 image.Image, sample int) float64 {
	b := img1.Bounds()
	w, h := b.Dx(), b.Dy()
	ox, oy := b.Min.X, b.Min.Y
	const (c1, c2 = 6.5025, 58.5225)
	var total, count float64
	step := 8 * sample
	for y := 0; y < h; y += step {
		for x := 0; x < w; x += step {
			var m1, m2, s1, s2, s12, n float64
			for by := y; by < y+8 && by < h; by++ {
				for bx := x; bx < x+8 && bx < w; bx++ {
					r1, g1, b1, _ := img1.At(ox+bx, oy+by).RGBA()
					r2, g2, b2, _ := img2.At(ox+bx, oy+by).RGBA()
					v1 := 0.299*float64(r1>>8) + 0.587*float64(g1>>8) + 0.114*float64(b1>>8)
					v2 := 0.299*float64(r2>>8) + 0.587*float64(g2>>8) + 0.114*float64(b2>>8)
					m1 += v1; m2 += v2; n++
				}
			}
			m1 /= n; m2 /= n
			for by := y; by < y+8 && by < h; by++ {
				for bx := x; bx < x+8 && bx < w; bx++ {
					r1, g1, b1, _ := img1.At(ox+bx, oy+by).RGBA()
					r2, g2, b2, _ := img2.At(ox+bx, oy+by).RGBA()
					v1 := 0.299*float64(r1>>8) + 0.587*float64(g1>>8) + 0.114*float64(b1>>8)
					v2 := 0.299*float64(r2>>8) + 0.587*float64(g2>>8) + 0.114*float64(b2>>8)
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

func formatSize(size int64) string {
	if size >= 1048576 {
		return fmt.Sprintf("%.2f MB", float64(size)/1048576)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
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
	stdDraw.BiLinear.Scale(small1, newRect, img1, b, stdDraw.Over, nil)
	stdDraw.BiLinear.Scale(small2, newRect, img2, b, stdDraw.Over, nil)

	dist, _ := butteraugli.CompareImages(small1, small2)
	return dist
}

func applyMetadata(src, dst string, keepAll, skipMeta bool) error {
	ext := strings.ToLower(filepath.Ext(src))
	if ext == ".jpg" || ext == ".jpeg" {
		return copyJPEGMetadata(src, dst, keepAll, skipMeta)
	}
	return nil
}

func isAlreadyProcessed(src string) bool {
	data, err := os.ReadFile(src)
	if err != nil { return false }
	
	// Scan JPEG markers for our APP15 signature
	for i := 0; i < len(data)-1; {
		if data[i] == 0xFF {
			marker := data[i+1]
			if marker == 0x00 || marker == 0xFF { i++; continue }
			if marker == 0xD8 { i += 2; continue }
			if marker == 0xDA || marker == 0xC0 || marker == 0xC2 { break } // Start of image data
			
			if i+3 >= len(data) { break }
			length := int(data[i+2])<<8 | int(data[i+3])
			
			if marker == 0xEF { // APP15
				if i+4+len(Signature) <= len(data) {
					if string(data[i+4:i+4+len(Signature)]) == Signature {
						return true
					}
				}
			}
			i += 2 + length
		} else {
			i++
		}
	}

	return false
}


func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil { return err }
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	srcInfo, _ := os.Stat(src)
	return os.Chmod(dst, srcInfo.Mode())
}

func moveFile(src, dst string) error {
	// Try atomic rename first
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Fallback for cross-device: copy then remove
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyJPEGMetadata(src, dst string, keepAll, skipMeta bool) (err error) {
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
							// Safety check for header access
							if i+2+33 <= len(srcData) {
								header := string(segment[4:33])
								if header == "http://ns.adobe.com/xmp/exten" {
									keep = false
								}
							}
						}
						// 2. Strip Photoshop thumbnails/binary data (APP13)
						if marker == 0xED && length > 14 {
							if i+2+14 <= len(srcData) {
								header := string(segment[4:14])
								if header == "Photoshop " {
									keep = false
								}
							}
						}
						// 3. Strip FPXR (FlashPix) which is usually large and useless
						if marker == 0xE2 && length > 10 {
							if i+2+9 <= len(srcData) {
								header := string(segment[4:9])
								if header == "FPXR" {
									keep = false
								}
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
	
	// Ensure JFIF (APP0) stays first if present among segments
	for i, seg := range segments {
		if len(seg) > 1 && seg[1] == 0xE0 {
			out.Write(seg)
			// Remove from slices to not duplicate later
			segments = append(segments[:i], segments[i+1:]...)
			break
		}
	}

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

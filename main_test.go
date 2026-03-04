package main

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// makeNRGBA creates a flat-color NRGBA image for testing.
func makeNRGBA(w, h int, r, g, b uint8) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}

// makeNoise creates an NRGBA image with a repeating pattern.
func makeNoise(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8((x*13 + y*7) % 256)
			img.SetNRGBA(x, y, color.NRGBA{R: v, G: 255 - v, B: uint8(x % 256), A: 255})
		}
	}
	return img
}

// --- toNRGBA ---

func TestToNRGBA_PreservesPixels(t *testing.T) {
	src := makeNRGBA(4, 4, 100, 150, 200)
	// toNRGBA on an *image.NRGBA should return an equivalent image
	result := toNRGBA(src)
	b := result.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			got := result.NRGBAAt(x, y)
			if got.R != 100 || got.G != 150 || got.B != 200 || got.A != 255 {
				t.Fatalf("pixel (%d,%d): got %v, want {100,150,200,255}", x, y, got)
			}
		}
	}
}

func TestToNRGBA_FromYCbCr(t *testing.T) {
	// Build a YCbCr image and verify toNRGBA round-trips correctly
	ycc := image.NewYCbCr(image.Rect(0, 0, 2, 2), image.YCbCrSubsampleRatio420)
	for i := range ycc.Y  { ycc.Y[i] = 200 }
	for i := range ycc.Cb { ycc.Cb[i] = 128 }
	for i := range ycc.Cr { ycc.Cr[i] = 128 }
	result := toNRGBA(ycc)
	if result == nil {
		t.Fatal("toNRGBA returned nil")
	}
	if result.Bounds() != ycc.Bounds() {
		t.Fatalf("bounds mismatch: got %v, want %v", result.Bounds(), ycc.Bounds())
	}
}

// --- getAdaptiveSample ---

func TestGetAdaptiveSample_SmallImage(t *testing.T) {
	// 100×100 = 10 000 px (≤1MP) → sample=1 (check every pixel)
	b := image.Rect(0, 0, 100, 100)
	got := getAdaptiveSample(b, false)
	if got != 1 {
		t.Fatalf("expected 1 for tiny image, got %d", got)
	}
}

func TestGetAdaptiveSample_1MP(t *testing.T) {
	// 1000×1000 = 1MP (≤1MP) → sample=1
	b := image.Rect(0, 0, 1000, 1000)
	got := getAdaptiveSample(b, false)
	if got != 1 {
		t.Fatalf("expected 1 for ~1MP image, got %d", got)
	}
}

func TestGetAdaptiveSample_4MP(t *testing.T) {
	// 2000×2000 = 4MP (≤4MP) → sample=2
	b := image.Rect(0, 0, 2000, 2000)
	got := getAdaptiveSample(b, false)
	if got != 2 {
		t.Fatalf("expected 2 for ~4MP image, got %d", got)
	}
}

func TestGetAdaptiveSample_16MP(t *testing.T) {
	// 4000×4000 = 16MP (≤16MP) → sample=4
	b := image.Rect(0, 0, 4000, 4000)
	got := getAdaptiveSample(b, false)
	if got != 4 {
		t.Fatalf("expected 4 for ~16MP image, got %d", got)
	}
}

func TestGetAdaptiveSample_LargeImage(t *testing.T) {
	// 8000×8000 = 64MP (≤64MP) → sample=8
	b := image.Rect(0, 0, 8000, 8000)
	got := getAdaptiveSample(b, false)
	if got != 8 {
		t.Fatalf("expected 8 for large image, got %d", got)
	}
}

func TestGetAdaptiveSample_HugeImage(t *testing.T) {
	// 12000×12000 = 144MP (>128MP) → return 0 (skip)
	b := image.Rect(0, 0, 12000, 12000)
	got := getAdaptiveSample(b, false)
	if got != 0 {
		t.Fatalf("expected 0 (skip) for >128MP image, got %d", got)
	}
}

// --- calculateMSEAndPSNR ---

func TestCalculateMSEAndPSNR_Identical(t *testing.T) {
	img := makeNRGBA(64, 64, 128, 64, 32)
	mse, psnr := calculateMSEAndPSNR(img, img, 1)
	if mse != 0 {
		t.Fatalf("MSE of identical images should be 0, got %f", mse)
	}
	if psnr != 100 {
		t.Fatalf("PSNR of identical images should be 100, got %f", psnr)
	}
}

func TestCalculateMSEAndPSNR_Different(t *testing.T) {
	img1 := makeNRGBA(64, 64, 128, 128, 128)
	img2 := makeNRGBA(64, 64, 0, 0, 0)
	mse, psnr := calculateMSEAndPSNR(img1, img2, 1)
	if mse <= 0 {
		t.Fatalf("MSE should be > 0 for different images, got %f", mse)
	}
	if psnr < 0 || psnr > 100 {
		t.Fatalf("PSNR out of range [0,100]: got %f", psnr)
	}
	// MSE is normalized: rawMSE=128²=16384 → mse=16384/65025≈0.252; PSNR≈5.93
	expectedMSE := 128.0 * 128.0 / (255.0 * 255.0)
	if math.Abs(mse-expectedMSE) > 0.001 {
		t.Fatalf("expected MSE ≈ %.4f, got %.4f", expectedMSE, mse)
	}
}

// --- calculatePSNR / calculateMSE ---

func TestCalculatePSNR_Identical(t *testing.T) {
	img := makeNoise(32, 32)
	psnr := calculatePSNR(img, img, 1)
	if psnr != 100 {
		t.Fatalf("PSNR of identical images should be 100, got %f", psnr)
	}
}

func TestCalculateMSE_Identical(t *testing.T) {
	img := makeNoise(32, 32)
	mse := calculateMSE(img, img, 1)
	if mse != 0 {
		t.Fatalf("MSE of identical images should be 0, got %f", mse)
	}
}

// --- calculateSSIM ---

func TestCalculateSSIM_Identical(t *testing.T) {
	img := makeNoise(64, 64)
	ssim := calculateSSIM(img, img, 1)
	if math.Abs(ssim-1.0) > 1e-9 {
		t.Fatalf("SSIM of identical images should be 1.0, got %f", ssim)
	}
}

func TestCalculateSSIM_Different(t *testing.T) {
	img1 := makeNRGBA(64, 64, 255, 255, 255)
	img2 := makeNRGBA(64, 64, 0, 0, 0)
	ssim := calculateSSIM(img1, img2, 1)
	if ssim > 0.5 {
		t.Fatalf("SSIM of black vs white should be low (<0.5), got %f", ssim)
	}
}

// --- sampling ---

func TestCalculateMSEAndPSNR_Sampling(t *testing.T) {
	// sample=2 should produce same result as sample=1 for identical images
	img := makeNRGBA(64, 64, 100, 100, 100)
	mse1, psnr1 := calculateMSEAndPSNR(img, img, 1)
	mse2, psnr2 := calculateMSEAndPSNR(img, img, 2)
	if mse1 != mse2 || psnr1 != psnr2 {
		t.Fatalf("identical images: sample=1 (%f,%f) != sample=2 (%f,%f)", mse1, psnr1, mse2, psnr2)
	}
}

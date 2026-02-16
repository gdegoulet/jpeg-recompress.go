# jpeg-recompress.go

`jpeg-recompress.go` is a high-performance command-line utility written in Go for recompressing JPEG images. It aims to minimize file size while maintaining a strictly controlled visual quality level using industry-standard metrics.

## Tools

This project provides two main tools:

1.  **`jpeg-recompress.go`**: An intelligent recompressor that automatically finds the best quality setting to reach a target visual threshold.
2.  **`jpegli-encode.go`**: A direct encoder using the Jpegli library for high-efficiency JPEG encoding at a fixed quality level.

---

## jpeg-recompress.go

## Features

- **Intelligent Recompression**: Uses a binary search algorithm to find the optimal compression level that satisfies your quality requirements.
- **Multiple Quality Metrics**:
    - **PSNR (Peak Signal-to-Noise Ratio)**: Default metric, good for general purpose.
    - **SSIM (Structural Similarity Index)**: Better reflects human visual perception.
    - **MSE (Mean Squared Error)**: Measures the average squared difference between pixels.
    - **Butteraugli**: Advanced psychovisual metric by Google (most accurate, but slow).
- **Adaptive Sub-sampling**: Automatically adjusts pixel sampling (1x to 32x) based on image resolution to ensure fast processing of high-resolution images without compromising metric accuracy.
- **Native Metadata Management**: 
    - Handles JPEG APP segments (EXIF, IPTC, XMP) natively in Go.
    - Preserves file permissions and modification times.
    - No external tools like `exiftool` or `perl` required.
- **JSON-First Output**: Designed for easy integration into pipelines, providing comprehensive statistics and verification results.
- **Safety Checks**: Verifies that the output is indeed smaller or equal to the input and ensures file integrity.
- **Idempotency**: Adds a `jpeg-recompress.go` signature in a private `APP15` JPEG segment to prevent redundant processing and generation loss without cluttering standard metadata fields like Software or Comment.

## How it Works

1.  **Binary Search for Quality**: The tool doesn't just "compress" the image; it searches for the lowest possible quality setting (between `min-quality` and `max-quality`) that still meets your target metric threshold (`PSNR`, `SSIM`, or `MSE`).
2.  **Adaptive Sampling**: For large images, calculating metrics on every single pixel is slow. `jpeg-recompress.go` uses a resolution-aware sampling strategy to maintain high performance while keeping metric accuracy within acceptable margins.
3.  **Metadata Preservation**: The tool extracts original APP segments from the source and reapplies them to the recompressed file.
4.  **Atomic Operations**: Recompression is performed on a temporary file. The original file is only replaced if the recompression is successful and the resulting file is smaller than the original.

## Build

**Standard build:**
```bash
GOAMD64=v3 go build -ldflags="-s -w" -o jpeg-recompress.go .
```

**100% Static Build (musl via Docker):**
Recommended for maximum portability across different Linux distributions.
```bash
./build-static.sh
```
This will produce a `jpeg-recompress.go` binary with zero dynamic dependencies.

## Usage

```bash
./jpeg-recompress.go -input <file> [options]
```

### Options

| Option | Description | Default |
| :--- | :--- | :--- |
| `-input` | **(Required)** Path to the source image. | |
| `-output` | Path to destination. If omitted, overwrites input. | Input path |
| `-metric` | Quality metric: `psnr`, `ssim`, `mse`. | `psnr` |
| `-threshold` | Target quality threshold. | `38.5` (STD), `42.0` (Jpegli) |
| `-min-quality` | Minimum quality level to attempt. | `70` |
| `-max-quality` | Maximum quality level to attempt. | `90` |
| `-sample` | Sub-sampling rate (1=every pixel, 0=auto). | `0` (Adaptive) |
| `-jpegli` | Use Jpegli encoder for superior compression (up to 35% better, **experimental**). Forces `-metric butteraugli`. | `false` |
| `-fast` | Step-based search (step=2) for faster execution. | `false` |
| `-keep-all-metadata` | Preserve all original metadata tags. | `false` |
| `-skip-metadata` | Remove all metadata (except signature). | `false` |
| `-quiet` | Suppress all output except errors. | `false` |
| `-debug` | Show detailed trace of the search process. | `false` |

---

## jpegli-encode.go

`jpegli-encode.go` is a simple and efficient tool to encode images directly using the [Jpegli](https://github.com/google/jpegli) library at a specific quality level.

### Features

- **High-Efficiency Encoding**: Leverages Jpegli's advanced psychovisual optimizations for better quality-to-size ratios.
- **Metadata Filtering**: Automatically strips "heavy" and non-essential metadata (Extended XMP, Photoshop previews, FPXR) while preserving critical tags (EXIF, IPTC, ICC Profiles).
- **Unix Integration**: Preserves original file permissions and modification times.
- **Safe Operations**: Uses atomic writes via temporary files.

### Usage

```bash
./jpegli-encode.go -input <file> [-output <file>] [-quality <1-100>]
```

| Option | Description | Default |
| :--- | :--- | :--- |
| `-input` | **(Required)** Path to the source image (JPEG, PNG, GIF). | |
| `-output` | Path to destination. If omitted, overwrites input. | Input path |
| `-quality` | Target encoding quality (1 to 100). | `90` |
| `-chroma_subsampling` | Chroma subsampling: `444`, `422`, `420`. | `420` |

### Example output

```text
Successfully encoded image.jpg to image.jpg (quality 85)
Size: 4.12 MB -> 2.56 MB (Gain: 37.8%)
```

---

## Quality Metrics Reference

Choosing the right threshold depends on your balance between file size and visual fidelity.

### PSNR (Peak Signal-to-Noise Ratio)
*Higher is better. Best for general purpose.*

| Usage | Threshold | Visual Quality |
| :--- | :--- | :--- |
| **Archivage / Pro** | **38.5 dB** | Secure, indistinguishable from original. |
| **Standard / Web HD** | **37.5 dB** | Excellent balance, good reduction. |
| **Aggressive Web** | **36.5 dB** | Significant gains, quality remains clean. |

### SSIM (Structural Similarity)
*Higher is better. Best for matching human perception.*

| Usage | Threshold | Visual Quality |
| :--- | :--- | :--- |
| **Archivage / Pro** | **0.995** | Perfect structure, no visible loss. |
| **Standard / Web HD** | **0.990** | High fidelity, standard for HD web. |
| **Aggressive Web** | **0.980** | Great for mobile/social media. |

### MSE (Mean Squared Error)
*Lower is better. Mathematical pixel-to-pixel difference.*

| Usage | Threshold | Visual Quality |
| :--- | :--- | :--- |
| **Archivage / Pro** | **0.00005** | Mathematical near-identity. |
| **Standard / Web HD** | **0.00010** | Professional grade. |
| **Aggressive Web** | **0.00050** | Acceptable noise for web assets. |

### Butteraugli
*Lower is better. Perceptual distance.*

| Usage | Threshold | Visual Quality |
| :--- | :--- | :--- |
| **Archivage / Pro** | **1.0** | Visually lossless. |
| **Standard / Web HD** | **1.5** | High fidelity. |
| **Aggressive Web** | **2.0** | Clean, but noticeable changes. |

## Benchmark: Standard vs Jpegli

Performance comparison using default settings: **Standard (PSNR 38.5)** vs **Jpegli (Butteraugli 1.0)**.

| Image | Standard Gain | Jpegli Gain | Difference |
| :--- | :--- | :--- | :--- |
| `00093.jpg` | 39.1% | **51.2%** | **+12.1%** |
| `00094.jpg` | 53.3% | **58.2%** | **+4.9%** |
| `test.jpg` | 7.1% | **13.3%** | **+6.2%** |
| `00012.jpg` | 32.4% | **39.0%** | **+6.6%** |
| `example.jpg` | 0% | **67.9%** | **+67.9%** |

*Note: Jpegli offers significantly better compression ratios while maintaining indistinguishable visual quality by focusing on perceptual rather than purely mathematical accuracy.*

## Validation

A validation script is provided to verify metadata preservation and idempotency across different modes:
```bash
./validate-jpeg.sh
```

## Exit Codes

| Code | Status | Description |
| :--- | :--- | :--- |
| **0** | **SUCCESS** | Successfully recompressed, skipped (idempotency), or copied (no gain possible with separate output). |
| **1** | **FAILURE** | Critical error, invalid input, or recompression produced a larger file without a separate output path. |

## Disclaimer

**IMPORTANT: This tool performs in-place modifications when no separate output path is specified.**

- **No Backups**: `jpeg-recompress.go` does **NOT** create automatic backups of your source images. If you overwrite your input files, the original data is permanently lost.
- **Responsibility**: You are solely responsible for any data loss or damage resulting from the use of this tool. It is highly recommended to test on a copy of your data or always specify a separate output directory.
- **Warranty**: This software is provided "as is", without warranty of any kind, express or implied.

## License

This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

#!/bin/bash

BINARY="./jpeg-recompress.go"
SRC_DIR="images"
OUT_DIR="out_compare"

if [ ! -f "$BINARY" ]; then
    echo "Building binary..."
    ./build-static.sh
fi

rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

echo "# Standard (PSNR 38.5) vs Jpegli (Butteraugli 1.0) Comparison"
echo "| File | Mode | Metric | Size (Bytes) | Gain vs Orig | Quality (Q) |"
echo "| :--- | :--- | :--- | :--- | :--- | :--- |"

find "$SRC_DIR" -type f \( -iname "*.jpg" -o -iname "*.jpeg" \) | while read img; do
    filename=$(basename "$img")
    
    # Skip non-images or very small files
    orig_size=$(stat -c%s "$img")
    if [ $orig_size -lt 1000 ]; then continue; fi

    echo "| $filename | **Original** | - | $orig_size | - | - |"
    
    # 1. Standard (Default PSNR 38.5)
    std_out=$($BINARY -input "$img" -output "$OUT_DIR/std_$filename" 2>/dev/null)
    std_size=$(stat -c%s "$OUT_DIR/std_$filename")
    std_gain=$(echo "$std_out" | jq -r '.gain_percent')
    std_q=$(echo "$std_out" | jq -r '.best_q')
    echo "| | Standard | PSNR | $std_size | $std_gain% | $std_q |"
    
    # 2. Jpegli (Default Butteraugli 1.0)
    li_out=$($BINARY -input "$img" -output "$OUT_DIR/li_$filename" -jpegli 2>/dev/null)
    li_size=$(stat -c%s "$OUT_DIR/li_$filename")
    li_gain=$(echo "$li_out" | jq -r '.gain_percent')
    li_q=$(echo "$li_out" | jq -r '.best_q')
    echo "| | **Jpegli** | **Butteraugli** | **$li_size** | **$li_gain%** | $li_q |"
    
    echo "| --- | --- | --- | --- | --- | --- |"
done

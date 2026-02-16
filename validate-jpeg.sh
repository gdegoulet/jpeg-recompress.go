#!/bin/bash

# Configuration
BINARY="./jpeg-recompress.go"
SRC_DIR="images"
OUT_DIR="out_validate"
REPORT="validation_report.md"

# Ensure binary exists
if [ ! -f "$BINARY" ]; then
    echo "Error: $BINARY not found. Run ./build-static.sh first."
    exit 1
fi

# Prepare environment
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

echo "# Validation Report - JPEG Recompress Branch" > "$REPORT"
echo "Date: $(date)" >> "$REPORT"
echo "" >> "$REPORT"
echo "| File | Mode | Status | Size (In -> Out) | Gain | Metadata (In -> Out) | Idempotence |" >> "$REPORT"
echo "| :--- | :--- | :--- | :--- | :--- | :--- | :--- |" >> "$REPORT"

format_size() {
    local size=$1
    if [ $size -ge 1048576 ]; then
        echo "$(echo "scale=2; $size / 1048576" | bc) MB"
    else
        echo "$(echo "scale=0; $size / 1024" | bc) KB"
    fi
}

check_metadata() {
    local orig="$1"
    local target="$2"
    local mode="$3"
    local status="$4"

    if ! command -v exiftool >/dev/null 2>&1; then
        echo "N/A (No exiftool)"
        return
    fi

    # Count real metadata tags (excluding system groups)
    local count_orig=$(exiftool -G -a -s "$orig" 2>/dev/null | grep -vE "\[File\]|\[ExifTool\]|\[Composite\]" | wc -l)
    local count_target=$(exiftool -G -a -s "$target" 2>/dev/null | grep -vE "\[File\]|\[ExifTool\]|\[Composite\]" | wc -l)
    local counts="($count_orig -> $count_target)"

    if [ "$status" == "COPIED_NO_GAIN" ] || [ "$status" == "SKIPPED" ]; then
        echo "✅ Preserved $counts"
        return
    fi

    if [ "$mode" == "Skip" ]; then
        if [ "$count_target" -lt 5 ]; then
            echo "✅ Stripped $counts"
        else
            echo "❌ Too many tags $counts"
        fi
    else
        # Compare critical tags
        local tags="Make Model DateTimeOriginal GPSPosition"
        local match=true
        for tag in $tags; do
            local val_orig=$(exiftool -s3 -"$tag" "$orig" 2>/dev/null)
            local val_target=$(exiftool -s3 -"$tag" "$target" 2>/dev/null)
            
            if [ -n "$val_orig" ] && [ "$val_orig" != "$val_target" ]; then
                match=false
                break
            fi
        done
        
        if [ "$match" == true ]; then
            echo "✅ Tags OK $counts"
        else
            echo "❌ Tag Mismatch $counts"
        fi
    fi
}

process_test() {
    local file="$1"
    local mode_name="$2"
    local extra_args="$3"
    local filename=$(basename "$file")
    local target="$OUT_DIR/${mode_name}_$filename"
    
    echo "Testing $filename in $mode_name mode..."
    
    # 1. Run recompression
    local result=$($BINARY -input "$file" -output "$target" $extra_args 2>/dev/null)
    local exit_code=$?
    
    if [ $exit_code -ne 0 ]; then
        if [ -f "$target" ]; then
             local status="COPIED_NO_GAIN"
        else
             echo "| $filename | $mode_name | **FAILED** | - | - | - | - |" >> "$REPORT"
             return
        fi
    else
        local status=$(echo "$result" | jq -r '.status')
    fi

    # 2. Extract size and gain info
    local s_in=$(stat -c%s "$file")
    local s_out=$(stat -c%s "$target")
    local size_str="$(format_size $s_in) -> $(format_size $s_out)"
    local gain="0%"
    if [ "$status" == "SUCCESS" ]; then
        gain=$(echo "$result" | jq -r '.gain_percent')"%"
    fi

    # 3. Metadata check
    local meta_status=$(check_metadata "$file" "$target" "$mode_name" "$status")

    # 4. Test Idempotency
    local second_run=$($BINARY -input "$target" -output "$OUT_DIR/tmp_$filename" $extra_args 2>/dev/null)
    local second_status=$(echo "$second_run" | jq -r '.status')
    local idempotence="❌"
    if [ "$second_status" == "COPIED_NO_GAIN" ] || [ "$second_status" == "SKIPPED" ]; then
        idempotence="✅"
    fi

    echo "| $filename | $mode_name | $status | $size_str | $gain | $meta_status | $idempotence |" >> "$REPORT"
}

# Main loop
find "$SRC_DIR" -maxdepth 1 -type f \( -iname "*.jpg" -o -iname "*.jpeg" \) | while read img; do
    # Skip non-images
    if [[ "$img" == *"not_an_image"* ]] || [[ "$img" == *"1x1"* ]]; then continue; fi
    
    process_test "$img" "Default" ""
    process_test "$img" "Skip" "-skip-metadata"
    process_test "$img" "KeepAll" "-keep-all-metadata"
done

echo "" >> "$REPORT"
echo "## Conclusion" >> "$REPORT"
echo "Report generated in $REPORT"
echo "Check $OUT_DIR for output files."

cat "$REPORT"

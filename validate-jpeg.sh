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
echo "| File | Mode | Status | Meta Count (In -> Out) | Idempotence |" >> "$REPORT"
echo "| :--- | :--- | :--- | :--- | :--- |" >> "$REPORT"

# Function to get metadata count using the binary itself (countMetadata is exposed via JSON output)
get_meta_count() {
    local file=$1
    # We use exiftool for validation purpose in the script if available, 
    # otherwise we'd need to parse the JSON output of the tool.
    if command -v exiftool >/dev/null 2>&1; then
        exiftool -all "$file" | wc -l
    else
        echo "?"
    fi
}

process_test() {
    local file=$1
    local mode_name=$2
    local extra_args=$3
    local filename=$(basename "$file")
    local target="$OUT_DIR/${mode_name}_$filename"
    
    echo "Testing $filename in $mode_name mode..."
    
    # 1. Run recompression
    local result=$($BINARY -input "$file" -output "$target" $extra_args 2>/dev/null)
    local exit_code=$?
    
    if [ $exit_code -ne 0 ]; then
        echo "| $filename | $mode_name | **FAILED** | - | - |" >> "$REPORT"
        return
    fi

    # 2. Extract info from JSON output
    local meta_in=$(echo "$result" | jq -r '.test_results.input_metadata_count')
    local meta_out=$(echo "$result" | jq -r '.test_results.output_metadata_count')
    local status=$(echo "$result" | jq -r '.status')

    # 3. Test Idempotency (run again on the output)
    local second_run=$($BINARY -input "$target" -output "$OUT_DIR/tmp_$filename" $extra_args 2>/dev/null)
    local second_status=$(echo "$second_run" | jq -r '.status')
    local idempotence="❌"
    if [ "$second_status" == "COPIED_NO_GAIN" ] || [ "$second_status" == "SKIPPED" ]; then
        idempotence="✅"
    fi

    echo "| $filename | $mode_name | $status | $meta_in -> $meta_out | $idempotence |" >> "$REPORT"
}

# Main loop
find "$SRC_DIR" -type f \( -iname "*.jpg" -o -iname "*.jpeg" \) | while read img; do
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

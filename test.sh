#!/bin/bash
rm -f out/*

echo "=== Testing Default Recompression ==="
find images -type f \( -iname "*.jpg" -o -iname "*.jpeg" \) | grep -v '1x1' | grep -v 'not_an_image' | cut -d '/' -f2 | while read f
do
./jpeg-recompress.go -input images/$f -output out/$f $@
echo $?
done

echo
echo "=== Testing Metadata: SKIP ==="
./jpeg-recompress.go -input images/example-original.jpg -output out/meta_skip.jpg -skip-metadata
echo $?
./jpeg-recompress.go -input images/PXL_20260121_111839477.jpg -output out/meta_skip2.jpg -skip-metadata
echo $?

echo
echo "=== Testing Metadata: KEEP ALL ==="
./jpeg-recompress.go -input images/example-original.jpg -output out/meta_keep.jpg -keep-all-metadata
echo $?
./jpeg-recompress.go -input images/PXL_20260121_111839477.jpg -output out/meta_keep2.jpg -keep-all-metadata
echo $?

echo
echo "=== Edge Case: 1x1 Image (SSIM check) ==="
./jpeg-recompress.go -input images/1x1.jpg -output out/1x1.jpg -metric ssim
echo $?

echo
echo "=== Edge Case: Invalid Image File ==="
./jpeg-recompress.go -input images/not_an_image.jpg -output out/invalid.jpg
echo $?

echo
echo "=== Clean-up Check (No .tmp files) ==="
ls -R | grep ".tmp_recompress" || echo "OK: No temporary files leaked."

echo
ls -alhrt out/

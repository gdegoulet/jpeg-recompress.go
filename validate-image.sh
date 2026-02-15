#!/bin/bash
f="00093"
test "$1" != "" && f="$1"

test ! -d images && exit 1
test ! -f "images/$f.jpg" && exit 1
test ! -d out && mkdir out
rm -f out/*

echo "### MSE ###############################################################"
ls -alh "images/$f.jpg"
./jpeg-recompress.go -input "images/$f.jpg" -output "out/$f.mse.jpg" -metric mse -debug | jq .
echo $?
ls -alh "out/$f.mse.jpg"
echo

echo "### SIMM ###############################################################"
ls -alh "images/$f.jpg"
./jpeg-recompress.go -input "images/$f.jpg" -output "out/$f.simm.jpg" -metric ssim -debug | jq .
echo $?
ls -alh "out/$f.simm.jpg"
echo

echo "### PSNR (default) ###############################################################"
ls -alh "images/$f.jpg"
./jpeg-recompress.go -input "images/$f.jpg" -output "out/$f.psnr.jpg" -debug | jq .
echo $?
ls -alh "out/$f.psnr.jpg"
echo

echo "### PSNR (default) FAST ###############################################################"
ls -alh "images/$f.jpg"
./jpeg-recompress.go -input "images/$f.jpg" -output "out/$f.psnr.fast.jpg" -fast -debug | jq .
echo $?
ls -alh "out/$f.psnr.fast.jpg"
echo



echo "### keep-all-metadata ###############################################################"
ls -alh "images/$f.jpg"
./jpeg-recompress.go -input "images/$f.jpg" -output "out/$f.keep-all-metadata.jpg" -keep-all-metadata -debug | jq .
echo $?
ls -alh "out/$f.keep-all-metadata.jpg"
echo

echo "### skip-metadata ###############################################################"
ls -alh "images/$f.jpg"
./jpeg-recompress.go -input "images/$f.jpg" -output "out/$f.skip-metadata.jpg" -skip-metadata -debug | jq .
echo $?
ls -alh "out/$f.skip-metadata.jpg"
echo

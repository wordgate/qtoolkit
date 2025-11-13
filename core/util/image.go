package util

import (
	"context"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

func ImageToPng(ctx context.Context, imageBytes []byte) ([]byte, error) {
	var img image.Image
	var err error

	contentType := http.DetectContentType(imageBytes)
	switch contentType {
	case "image/png":
		return imageBytes, nil
	case "image/webp":
		img, err = webp.Decode(bytes.NewReader(imageBytes))
	case "image/jpeg":
		// Decode the PNG image bytes
		img, err = jpeg.Decode(bytes.NewReader(imageBytes))
	default:
		return nil, fmt.Errorf("unable to convert %#v to jpeg", contentType)
	}

	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func ImageToJpeg(ctx context.Context, imageBytes []byte) ([]byte, error) {
	var img image.Image
	var err error

	contentType := http.DetectContentType(imageBytes)
	switch contentType {
	case "image/jpeg":
		return imageBytes, nil
	case "image/webp":
		img, err = webp.Decode(bytes.NewReader(imageBytes))
	case "image/png":
		// Decode the PNG image bytes
		img, err = png.Decode(bytes.NewReader(imageBytes))
	default:
		return nil, fmt.Errorf("unable to convert %#v to jpeg", contentType)
	}

	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	// encode the image as a JPEG file
	if err := jpeg.Encode(buf, img, nil); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func ImageAsBase64(ctx context.Context, imageBytes []byte) (string, error) {
	bt, err := ImageToPng(ctx, imageBytes)
	if err != nil {
		return "", err
	}
	s := base64.StdEncoding.EncodeToString(bt)
	return "data:image/png;base64," + s, nil
}

func ImageFromBase64(ctx context.Context, b64 string) ([]byte, error) {
	s := strings.TrimPrefix(b64, "data:image/png;base64,")
	if s == b64 {
		return nil, fmt.Errorf("string is not start with 'data:base64/png;base64,'")
	}
	return base64.StdEncoding.DecodeString(s)
}

func ImageIsBase64(b64 string) bool {
	s := strings.TrimPrefix(b64, "data:image/png;base64,")
	return s != b64
}

func ImageResizeThenCorp(img image.Image, size int) image.Image {
	// Determine the new dimensions preserving aspect ratio
	var newWidth, newHeight int
	if img.Bounds().Dx() > img.Bounds().Dy() {
		newHeight = size
		newWidth = img.Bounds().Dx() * size / img.Bounds().Dy()
	} else {
		newWidth = size
		newHeight = img.Bounds().Dy() * size / img.Bounds().Dx()
	}

	// Resize the image
	resizedImg := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.BiLinear.Scale(resizedImg, resizedImg.Bounds(), img, img.Bounds(), draw.Over, nil)

	// Center crop the image to a square
	cropRect := image.Rect(
		(newWidth-size)/2,
		(newHeight-size)/2,
		(newWidth+size)/2,
		(newHeight+size)/2,
	)
	croppedImg := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(croppedImg, croppedImg.Bounds(), resizedImg, cropRect.Min, draw.Src)

	return croppedImg
}
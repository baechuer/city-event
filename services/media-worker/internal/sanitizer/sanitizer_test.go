package sanitizer

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
)

func createTestImage(format string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Fill with some color
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	var buf bytes.Buffer
	if format == "jpeg" {
		jpeg.Encode(&buf, img, nil)
	} else if format == "png" {
		png.Encode(&buf, img)
	}
	return buf.Bytes()
}

func TestDetectType(t *testing.T) {
	// JPEG
	jpgData := createTestImage("jpeg")
	typ, err := DetectType(jpgData)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", typ)

	// PNG
	pngData := createTestImage("png")
	typ, err = DetectType(pngData)
	assert.NoError(t, err)
	assert.Equal(t, "image/png", typ)

	// Invalid
	typ, err = DetectType([]byte("invalid data"))
	assert.Error(t, err)
}

func TestProcess_Resize(t *testing.T) {
	pngData := createTestImage("png")

	sizes := []ResizeConfig{
		{Width: 50, Height: 50, Crop: true},
	}

	results, err := Process(pngData, sizes, 1000, 1000)
	assert.NoError(t, err)
	assert.NotNil(t, results)

	// Check output
	assert.Contains(t, results, "50")

	// Verify size (Sanitizer currently encodes as JPEG in EncodeWebP stub)
	res50 := results["50"]
	// Attempt decode as JPEG
	img50, err := jpeg.Decode(bytes.NewReader(res50))
	assert.NoError(t, err)
	assert.Equal(t, 50, img50.Bounds().Dx())
	assert.Equal(t, 50, img50.Bounds().Dy())
}

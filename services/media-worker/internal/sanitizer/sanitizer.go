package sanitizer

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// AllowedMagicBytes defines magic bytes for allowed image types.
var AllowedMagicBytes = map[string][]byte{
	"image/jpeg": {0xFF, 0xD8, 0xFF},
	"image/png":  {0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
	"image/webp": {0x52, 0x49, 0x46, 0x46}, // RIFF header (WebP starts with RIFF....WEBP)
}

// DetectType detects the actual image type from magic bytes.
func DetectType(data []byte) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("data too short to detect type")
	}

	// Check JPEG
	if bytes.HasPrefix(data, AllowedMagicBytes["image/jpeg"]) {
		return "image/jpeg", nil
	}

	// Check PNG
	if bytes.HasPrefix(data, AllowedMagicBytes["image/png"]) {
		return "image/png", nil
	}

	// Check WebP (RIFF....WEBP)
	if bytes.HasPrefix(data, AllowedMagicBytes["image/webp"]) && string(data[8:12]) == "WEBP" {
		return "image/webp", nil
	}

	return "", fmt.Errorf("unsupported image type")
}

// DecodeImage decodes an image from raw bytes.
func DecodeImage(data []byte, mimeType string) (image.Image, error) {
	reader := bytes.NewReader(data)

	switch mimeType {
	case "image/jpeg":
		return jpeg.Decode(reader)
	case "image/png":
		return png.Decode(reader)
	case "image/webp":
		return webp.Decode(reader)
	default:
		return nil, fmt.Errorf("unsupported image type: %s", mimeType)
	}
}

// ResizeConfig defines how to resize an image.
type ResizeConfig struct {
	Width  int
	Height int
	Crop   bool // If true, crop to exact dimensions; if false, preserve aspect ratio
}

// Resize resizes an image according to the config.
func Resize(img image.Image, cfg ResizeConfig) image.Image {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	var dstW, dstH int

	if cfg.Crop {
		// Crop to exact dimensions (center crop)
		dstW = cfg.Width
		dstH = cfg.Height

		// Calculate crop region
		aspectSrc := float64(srcW) / float64(srcH)
		aspectDst := float64(dstW) / float64(dstH)

		var cropRect image.Rectangle
		if aspectSrc > aspectDst {
			// Source is wider, crop horizontally
			newW := int(float64(srcH) * aspectDst)
			x := (srcW - newW) / 2
			cropRect = image.Rect(x, 0, x+newW, srcH)
		} else {
			// Source is taller, crop vertically
			newH := int(float64(srcW) / aspectDst)
			y := (srcH - newH) / 2
			cropRect = image.Rect(0, y, srcW, y+newH)
		}

		// Create cropped image
		cropped := image.NewRGBA(image.Rect(0, 0, cropRect.Dx(), cropRect.Dy()))
		draw.Draw(cropped, cropped.Bounds(), img, cropRect.Min, draw.Src)
		img = cropped
		srcW = cropped.Bounds().Dx()
		srcH = cropped.Bounds().Dy()
	} else {
		// Preserve aspect ratio
		if cfg.Height == 0 {
			// Only width specified
			ratio := float64(cfg.Width) / float64(srcW)
			dstW = cfg.Width
			dstH = int(float64(srcH) * ratio)
		} else if cfg.Width == 0 {
			// Only height specified
			ratio := float64(cfg.Height) / float64(srcH)
			dstH = cfg.Height
			dstW = int(float64(srcW) * ratio)
		} else {
			// Both specified, fit within box
			ratioW := float64(cfg.Width) / float64(srcW)
			ratioH := float64(cfg.Height) / float64(srcH)
			ratio := ratioW
			if ratioH < ratioW {
				ratio = ratioH
			}
			dstW = int(float64(srcW) * ratio)
			dstH = int(float64(srcH) * ratio)
		}
	}

	// Don't upscale
	if dstW > srcW || dstH > srcH {
		dstW = srcW
		dstH = srcH
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	return dst
}

// EncodeWebP encodes an image to WebP format.
// Note: golang.org/x/image/webp only supports decoding, not encoding.
// For production, you'd use a library like github.com/chai2010/webp
// For now, we'll encode as high-quality JPEG which is still safe.
func EncodeWebP(w io.Writer, img image.Image) error {
	// TODO: Replace with actual WebP encoding library
	return jpeg.Encode(w, img, &jpeg.Options{Quality: 85})
}

// Process processes an image: decode, validate, resize, and re-encode.
func Process(data []byte, sizes []ResizeConfig, maxWidth, maxHeight int) (map[string][]byte, error) {
	// Detect type from magic bytes
	mimeType, err := DetectType(data)
	if err != nil {
		return nil, fmt.Errorf("invalid image type: %w", err)
	}

	// Decode
	img, err := DecodeImage(data, mimeType)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Validate dimensions
	bounds := img.Bounds()
	if bounds.Dx() > maxWidth || bounds.Dy() > maxHeight {
		return nil, fmt.Errorf("image too large: %dx%d (max %dx%d)", bounds.Dx(), bounds.Dy(), maxWidth, maxHeight)
	}

	// Generate all sizes
	results := make(map[string][]byte)
	for _, size := range sizes {
		resized := Resize(img, size)

		var buf bytes.Buffer
		if err := EncodeWebP(&buf, resized); err != nil {
			return nil, fmt.Errorf("failed to encode size %dx%d: %w", size.Width, size.Height, err)
		}

		key := fmt.Sprintf("%d", size.Width)
		if size.Width == 0 {
			key = fmt.Sprintf("%d", size.Height)
		}
		results[key] = buf.Bytes()
	}

	return results, nil
}

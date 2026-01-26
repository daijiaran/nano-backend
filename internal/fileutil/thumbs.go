package fileutil

import (
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"math"
	"os"
	"time"
)

const (
	ThumbMaxEdge    = 512
	ThumbQuality    = 78
	ThumbMimeType   = "image/jpeg"
	thumbFileSuffix = ".thumb"
)

// ThumbPath returns the cached thumbnail path for the given original file path.
func ThumbPath(originalPath string) string {
	return fmt.Sprintf("%s%s-%d.jpg", originalPath, thumbFileSuffix, ThumbMaxEdge)
}

// RemoveWithThumb deletes the original file and its thumbnail (if any).
func RemoveWithThumb(originalPath string) {
	if originalPath == "" {
		return
	}
	_ = os.Remove(originalPath)
	_ = os.Remove(ThumbPath(originalPath))
}

// EnsureThumbnail returns a cached thumbnail path, generating it if needed.
func EnsureThumbnail(originalPath string) (string, error) {
	thumbPath := ThumbPath(originalPath)

	origInfo, err := os.Stat(originalPath)
	if err != nil {
		return "", err
	}

	if thumbInfo, err := os.Stat(thumbPath); err == nil {
		if thumbInfo.Size() > 0 && thumbInfo.ModTime().After(origInfo.ModTime().Add(-1*time.Second)) {
			return thumbPath, nil
		}
	}

	srcFile, err := os.Open(originalPath)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	srcImg, _, err := image.Decode(srcFile)
	if err != nil {
		return "", err
	}

	dstImg := resizeToMaxEdge(srcImg, ThumbMaxEdge)

	tmpPath := thumbPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}

	encodeErr := jpeg.Encode(out, dstImg, &jpeg.Options{Quality: ThumbQuality})
	closeErr := out.Close()
	if encodeErr != nil {
		_ = os.Remove(tmpPath)
		return "", encodeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", closeErr
	}

	_ = os.Remove(thumbPath)
	if err := os.Rename(tmpPath, thumbPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}

	return thumbPath, nil
}

func resizeToMaxEdge(src image.Image, maxEdge int) image.Image {
	b := src.Bounds()
	w := b.Dx()
	h := b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if maxEdge <= 0 {
		return src
	}

	scale := 1.0
	if w >= h {
		if w > maxEdge {
			scale = float64(maxEdge) / float64(w)
		}
	} else if h > maxEdge {
		scale = float64(maxEdge) / float64(h)
	}

	if scale >= 1.0 {
		return src
	}

	newW := int(math.Round(float64(w) * scale))
	newH := int(math.Round(float64(h) * scale))
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	for y := 0; y < newH; y++ {
		sy := int(float64(y) * float64(h) / float64(newH))
		for x := 0; x < newW; x++ {
			sx := int(float64(x) * float64(w) / float64(newW))
			dst.Set(x, y, src.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}

	return dst
}

//go:build cgo

package imaging

import (
	"image"
	"io"

	"github.com/chai2010/webp"
)

func encodeWebP(w io.Writer, img image.Image, quality int) error {
	return webp.Encode(w, img, &webp.Options{Quality: float32(quality)})
}

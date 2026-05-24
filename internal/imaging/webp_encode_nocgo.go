//go:build !cgo

package imaging

import (
	"fmt"
	"image"
	"io"
)

func encodeWebP(_ io.Writer, _ image.Image, _ int) error {
	return fmt.Errorf("webp encoding requires cgo-enabled builds")
}

// Package imageedit owns bounded local image copies and independent image locks.
package imageedit

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	_ "golang.org/x/image/webp"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
)

const MaxBytes = 10 << 20
const MaxPixels = 16_000_000
const DefaultRoot = "/data/native-media/images"

var ErrInvalid = errors.New("图片无效：仅支持 JPG、PNG、WebP，文件不超过10MB，单边不超过8192且总像素不超过1600万，不支持动画")
var ErrBusy = errors.New("图片处理繁忙，请稍后重试")
var decoders = make(chan struct{}, 2)

type Normalized struct {
	Body                   []byte
	ContentType, Extension string
	Width, Height          int
}

// Normalize holds a small global decode budget; excess work is rejected, never queued indefinitely.
func Normalize(ctx context.Context, name string, raw []byte) (out Normalized, err error) {
	select {
	case decoders <- struct{}{}:
		defer func() { <-decoders }()
	default:
		return out, ErrBusy
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	if len(raw) == 0 || len(raw) > MaxBytes {
		return out, ErrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	ext := strings.ToLower(filepath.Ext(name))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 || int64(cfg.Width)*int64(cfg.Height) > MaxPixels {
		return out, ErrInvalid
	}
	if !(format == "jpeg" && (ext == ".jpg" || ext == ".jpeg") || format == "png" && ext == ".png" || format == "webp" && ext == ".webp") || animated(raw, format) {
		return out, ErrInvalid
	}
	pic, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return out, ErrInvalid
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	var buf bytes.Buffer
	if format == "jpeg" {
		err = jpeg.Encode(&buf, pic, &jpeg.Options{Quality: 90})
		out.ContentType, out.Extension = "image/jpeg", "jpg"
	} else {
		err = png.Encode(&buf, pic)
		out.ContentType, out.Extension = "image/png", "png"
	}
	if err != nil || buf.Len() > MaxBytes {
		return Normalized{}, ErrInvalid
	}
	out.Body, out.Width, out.Height = buf.Bytes(), cfg.Width, cfg.Height
	return out, nil
}

// Walk bounded container chunks, not compressed pixel bytes (which may contain chunk names).
func animated(raw []byte, format string) bool {
	off := 8
	if format == "webp" {
		off = 12
	}
	if format == "jpeg" {
		return false
	}
	for off+8 <= len(raw) {
		var n uint32
		var kind string
		pad := 0
		if format == "png" {
			n = binary.BigEndian.Uint32(raw[off:])
			kind = string(raw[off+4 : off+8])
			pad = 4
		} else {
			n = binary.LittleEndian.Uint32(raw[off+4:])
			kind = string(raw[off : off+4])
			pad = int(n & 1)
		}
		if uint64(off)+8+uint64(n)+uint64(pad) > uint64(len(raw)) {
			return true
		}
		if kind == "acTL" || kind == "ANIM" || kind == "ANMF" {
			return true
		}
		if kind == "VP8X" && n > 0 && raw[off+8]&2 != 0 {
			return true
		}
		off += 8 + int(n) + pad
	}
	return off != len(raw)
}

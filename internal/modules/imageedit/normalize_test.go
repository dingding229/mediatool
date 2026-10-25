package imageedit

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func pngFixture(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	pic := image.NewNRGBA(image.Rect(0, 0, 12, 18))
	pic.Set(2, 2, color.NRGBA{255, 0, 0, 128})
	if e := png.Encode(&b, pic); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestNormalizeFormatsLimitsAndMetadata(t *testing.T) {
	ctx := context.Background()
	raw := pngFixture(t)
	var jpegFile bytes.Buffer
	if e := jpeg.Encode(&jpegFile, image.NewRGBA(image.Rect(0, 0, 16, 9)), nil); e != nil {
		t.Fatal(e)
	}
	// APP1 carries an identifying metadata string, which must not survive re-encoding.
	jpg := append([]byte{255, 216, 255, 225, 0, 14}, []byte("private-exif")...)
	jpg = append(jpg, jpegFile.Bytes()[2:]...)
	for _, tc := range []struct {
		name string
		raw  []byte
		ok   bool
	}{{"poster.png", raw, true}, {"poster.JPG", jpg, true}, {"fake.jpg", raw, false}, {"text.png", []byte("not an image"), false}, {"large.png", make([]byte, MaxBytes+1), false}, {"photo.svg", raw, false}} {
		t.Run(tc.name, func(t *testing.T) {
			out, e := Normalize(ctx, tc.name, tc.raw)
			if tc.ok {
				if e != nil || len(out.Body) == 0 || bytes.Contains(out.Body, []byte("private-exif")) {
					t.Fatal(out, e)
				}
				pic, _, e := image.Decode(bytes.NewReader(out.Body))
				if e != nil {
					t.Fatal(e)
				}
				if tc.name == "poster.png" {
					_, _, _, alpha := pic.At(2, 2).RGBA()
					if alpha == 65535 {
						t.Fatal("alpha lost")
					}
				}
			} else if !errors.Is(e, ErrInvalid) {
				t.Fatal(e)
			}
		})
	}
	huge := append([]byte(nil), raw...)
	binary.BigEndian.PutUint32(huge[16:20], 8193)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	if _, e := Normalize(ctx, "huge.png", huge); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	huge = append([]byte(nil), raw...)
	binary.BigEndian.PutUint32(huge[16:20], 8000)
	binary.BigEndian.PutUint32(huge[20:24], 8000)
	binary.BigEndian.PutUint32(huge[29:33], crc32.ChecksumIEEE(huge[12:29]))
	if _, e := Normalize(ctx, "pixels.png", huge); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	animatedPNG := append([]byte{}, raw[:33]...)
	animatedPNG = append(animatedPNG, []byte{0, 0, 0, 8, 'a', 'c', 'T', 'L', 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0}...)
	animatedPNG = append(animatedPNG, raw[33:]...)
	if _, e := Normalize(ctx, "animated.png", animatedPNG); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}
func TestNormalizeWebPAndBudget(t *testing.T) {
	// Use the pinned decoder's primary-source fixture, no network or new encoder.
	root := os.Getenv("GOMODCACHE")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, "go/pkg/mod")
	}
	raw, e := os.ReadFile(filepath.Join(root, "golang.org/x/image@v0.46.0/testdata/gopher-doc.1bpp.lossless.webp"))
	if e != nil {
		t.Fatal(e)
	}
	out, e := Normalize(context.Background(), "logo.webp", raw)
	if e != nil || out.ContentType != "image/png" {
		t.Fatal(e, out.ContentType)
	}
	decoders <- struct{}{}
	decoders <- struct{}{}
	defer func() { <-decoders; <-decoders }()
	if _, e := Normalize(context.Background(), "logo.webp", raw); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
}

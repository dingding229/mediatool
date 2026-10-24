package nativeplayback

import (
	"bytes"
	"testing"
)

func TestTextSubtitleBoundedException(t *testing.T) {
	body := []byte("1\n00:00:01,000 --> 00:00:02,000\ntext\n")
	v, e := NewTextSubtitle("srt", body)
	if e != nil || v.ContentType() != "application/x-subrip; charset=utf-8" || !bytes.Equal(v.Bytes(), body) {
		t.Fatal(v, e)
	}
	for _, c := range []struct {
		ext  string
		data []byte
	}{{"mkv", body}, {"srt", nil}, {"srt", []byte{0xff, 0}}, {"srt", bytes.Repeat([]byte("-->"), 2<<20)}, {"ass", body}} {
		if _, e := NewTextSubtitle(c.ext, c.data); e == nil {
			t.Fatal("unbounded media exception", c.ext)
		}
	}
}

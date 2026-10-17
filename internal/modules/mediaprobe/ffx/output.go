package ffx

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

type limitedOutput struct {
	mu       sync.Mutex
	data     []byte
	limit    int
	overflow bool
	stop     func() error
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	remaining := b.limit - len(b.data)
	if len(p) <= remaining {
		b.data = append(b.data, p...)
		b.mu.Unlock()
		return len(p), nil
	}
	b.data = append(b.data, p[:remaining]...)
	b.overflow = true
	b.mu.Unlock()
	return 0, errors.Join(ErrOutputLimit, b.stop())
}
func (b *limitedOutput) exceeded() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.overflow }
func (b *limitedOutput) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte{}, b.data...)
}

type tailOutput struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
}

func (t *tailOutput) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	size := len(p)
	if len(p) >= 4096 {
		t.data = append(t.data[:0], p[len(p)-4096:]...)
		t.truncated = true
	} else {
		t.data = append(t.data, p...)
		if len(t.data) > 4096 {
			t.data = append(t.data[:0], t.data[len(t.data)-4096:]...)
			t.truncated = true
		}
	}
	return size, nil
}

var remoteURL = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

func (t *tailOutput) safe(input source) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	raw := t.data
	// A cut first line may start halfway through a signature; never expose it.
	if t.truncated {
		if i := bytes.IndexByte(raw, '\n'); i >= 0 {
			raw = raw[i+1:]
		} else {
			return "[stderr tail truncated]"
		}
	}
	text := remoteURL.ReplaceAllStringFunc(strings.ToValidUTF8(string(raw), ""), func(value string) string {
		u, err := url.Parse(value)
		if err != nil {
			return "[remote URL]"
		}
		u.RawQuery = ""
		u.ForceQuery = false
		u.Fragment = ""
		u.User = nil
		return u.String()
	})
	for _, secret := range input.secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	if len(text) > 4096 {
		text = text[len(text)-4096:]
	}
	return strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' || r == 127 {
			return -1
		}
		return r
	}, strings.ToValidUTF8(text, ""))
}

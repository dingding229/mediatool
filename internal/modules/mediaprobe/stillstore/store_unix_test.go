//go:build linux || darwin

package stillstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"media-tool/internal/modules/mediaprobe/stillqueue"
)

const testAsset = "ab111111-1111-4111-8111-111111111111"

var testFP = strings.Repeat("a", 64)

func realTemp(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := filepath.Join(realTemp(t), "native-media", "stills")
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, root
}
func testJPEG(t *testing.T, c color.Color, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func imageFile(t *testing.T, root string) string {
	t.Helper()
	path, err := stillqueue.ImagePath(testAsset, testFP)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, filepath.FromSlash(path))
}
func assertNoTemporary(t *testing.T, root string) {
	t.Helper()
	entries, err := filepath.Glob(filepath.Join(root, "ab", ".tmp-*"))
	if err != nil || len(entries) != 0 {
		t.Fatal("temporary residue", len(entries), err)
	}
}
func TestRoundTripImmutableAndClosedStore(t *testing.T) {
	s, root := testStore(t)
	raw := testJPEG(t, color.RGBA{R: 180, A: 255}, 160, 90)
	receipt, err := s.Publish(t.Context(), testAsset, testFP, raw)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if receipt.RelativePath != "ab/"+testAsset+"-"+testFP[:16]+".jpg" || receipt.Digest != hex.EncodeToString(digest[:]) || receipt.Width != 160 || receipt.Height != 90 || receipt.SizeBytes != int64(len(raw)) {
		t.Fatal(receipt)
	}
	got, meta, err := s.Read(t.Context(), receipt.RelativePath)
	if err != nil || !bytes.Equal(got, raw) || meta != receipt {
		t.Fatal("roundtrip", err)
	}
	info, err := os.Stat(imageFile(t, root))
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Publish(t.Context(), testAsset, testFP, raw)
	if err != nil || again != receipt {
		t.Fatal("idempotence", err)
	}
	after, err := os.Stat(imageFile(t, root))
	if err != nil || !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("existing file rewritten")
	}
	other := testJPEG(t, color.Black, 160, 90)
	if _, err := s.Publish(t.Context(), testAsset, testFP, other); !errors.Is(err, ErrConflict) {
		t.Fatal("overwrote immutable image", err)
	}
	still, err := os.ReadFile(imageFile(t, root))
	if err != nil || !bytes.Equal(still, raw) {
		t.Fatal("conflict changed original")
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("non-private file", info.Mode())
	}
	for _, dir := range []string{root, filepath.Join(root, "ab")} {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatal("non-private directory", err)
		}
	}
	assertNoTemporary(t, root)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal("close not idempotent")
	}
	if _, _, err := s.Read(t.Context(), receipt.RelativePath); err != ErrUnavailable {
		t.Fatal("read closed", err)
	}
	if _, err := s.Publish(t.Context(), testAsset, testFP, raw); err != ErrUnavailable {
		t.Fatal("publish closed", err)
	}
}
func TestValidationBoundsAndContext(t *testing.T) {
	s, root := testStore(t)
	valid := testJPEG(t, color.Black, 16, 9)
	for _, raw := range [][]byte{nil, []byte("https://synthetic.invalid/?secret=NEVER_LOG"), bytes.Repeat([]byte("x"), MaxJPEGBytes+1), valid[:len(valid)/2], testJPEG(t, color.Black, 1281, 1)} {
		if _, err := s.Publish(t.Context(), testAsset, testFP, raw); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid JPEG accepted", err)
		}
	}
	// Large declared dimensions are refused before pixel allocation/decode.
	for _, dimensions := range [][2]int{{1280, 16384}, {1, 16385}} {
		raw := append([]byte(nil), valid...)
		at := bytes.Index(raw, []byte{0xff, 0xc0})
		if at < 0 {
			t.Fatal("synthetic SOF not found")
		}
		raw[at+5], raw[at+6] = byte(dimensions[1]>>8), byte(dimensions[1])
		raw[at+7], raw[at+8] = byte(dimensions[0]>>8), byte(dimensions[0])
		if _, err := s.Publish(t.Context(), testAsset, testFP, raw); !errors.Is(err, ErrInvalid) {
			t.Fatal("oversize header accepted", err)
		}
	}
	for _, path := range []string{"", DefaultRoot, "../x", "ab/../../outside.jpg", "ab\\leaf.jpg", "/ab/leaf.jpg", "https://synthetic.invalid/private"} {
		if _, _, err := s.Read(t.Context(), path); err != ErrUnsafe {
			t.Fatal("unsafe read path", err)
		}
	}
	badPath := "cd/" + testAsset + "-" + testFP[:16] + ".jpg"
	if _, _, err := s.Read(t.Context(), badPath); err != ErrUnsafe {
		t.Fatal("wrong shard accepted")
	}
	if _, err := s.Publish(t.Context(), strings.ToUpper(testAsset), testFP, valid); err != ErrInvalid {
		t.Fatal("noncanonical UUID")
	}
	if _, err := s.Publish(t.Context(), testAsset, "../private", valid); err != ErrInvalid {
		t.Fatal("unsafe fingerprint")
	}
	path, err := stillqueue.ImagePath(testAsset, testFP)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Read(t.Context(), path); err != ErrNotFound {
		t.Fatal("missing image", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Publish(ctx, testAsset, testFP, valid); err != context.Canceled {
		t.Fatal("publish canceled", err)
	}
	if _, _, err := s.Read(ctx, path); err != context.Canceled {
		t.Fatal("read canceled", err)
	}
	assertNoTemporary(t, root)
}
func TestRejectsEverySymlinkLayerAndHardlinks(t *testing.T) {
	for _, layer := range []string{"ancestor", "root", "shard-outside", "shard-inside", "file", "lock", "hardlink-file", "hardlink-lock"} {
		t.Run(layer, func(t *testing.T) {
			base := realTemp(t)
			outside := realTemp(t)
			sentinel := filepath.Join(outside, "sentinel.jpg")
			raw := testJPEG(t, color.Black, 16, 9)
			if err := os.WriteFile(sentinel, raw, 0600); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(base, "native-media", "stills")
			if layer == "ancestor" {
				if err := os.Symlink(outside, filepath.Join(base, "native-media")); err != nil {
					t.Fatal(err)
				}
				if _, err := Open(root); !errors.Is(err, ErrUnsafe) {
					t.Fatal("ancestor link followed", err)
				}
				if _, err := os.Lstat(filepath.Join(outside, "stills")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("created outside root")
				}
			} else if layer == "root" {
				if err := os.Mkdir(filepath.Dir(root), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, root); err != nil {
					t.Fatal(err)
				}
				if _, err := Open(root); !errors.Is(err, ErrUnsafe) {
					t.Fatal("root link followed", err)
				}
			} else {
				s, err := Open(root)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := s.Close(); err != nil {
						t.Error(err)
					}
				}()
				if strings.HasPrefix(layer, "shard") {
					target := outside
					if layer == "shard-inside" {
						target = filepath.Join(root, "00")
						if err := os.Mkdir(target, 0700); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.Symlink(target, filepath.Join(root, "ab")); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Mkdir(filepath.Join(root, "ab"), 0700); err != nil {
						t.Fatal(err)
					}
					target := imageFile(t, root)
					if strings.HasSuffix(layer, "lock") {
						target = filepath.Join(root, ".lock")
					}
					if strings.HasPrefix(layer, "hardlink") {
						if err := os.Link(sentinel, target); err != nil {
							t.Fatal(err)
						}
					} else {
						if err := os.Symlink(sentinel, target); err != nil {
							t.Fatal(err)
						}
					}
				}
				if _, err := s.Publish(t.Context(), testAsset, testFP, raw); !errors.Is(err, ErrUnsafe) {
					t.Fatal("unsafe entry accepted", err)
				}
				if !strings.HasSuffix(layer, "lock") {
					path, err := stillqueue.ImagePath(testAsset, testFP)
					if err != nil {
						t.Fatal(err)
					}
					if _, _, err := s.Read(t.Context(), path); !errors.Is(err, ErrUnsafe) {
						t.Fatal("unsafe entry read", err)
					}
				}
			}
			after, err := os.ReadFile(sentinel)
			if err != nil || !bytes.Equal(after, raw) {
				t.Fatal("outside sentinel changed")
			}
		})
	}
	for _, root := range []string{"stills", "/data", "/data/native-media/other", "/data/native-media/stills/", "/data/native-media/../stills"} {
		if _, err := Open(root); err != ErrUnsafe {
			t.Fatal("unsafe root", root, err)
		}
	}
}
func TestConcurrentPublishAndAtomicNoReplaceRace(t *testing.T) {
	s, root := testStore(t)
	second, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := second.Close(); err != nil {
			t.Error(err)
		}
	}()
	red := testJPEG(t, color.RGBA{R: 220, A: 255}, 32, 18)
	blue := testJPEG(t, color.RGBA{B: 220, A: 255}, 32, 18)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []stillqueue.Image
	var unexpected []error
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := s
			raw := red
			if i%2 == 1 {
				store = second
				raw = blue
			}
			receipt, err := store.Publish(t.Context(), testAsset, testFP, raw)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				results = append(results, receipt)
			} else if !errors.Is(err, ErrConflict) {
				unexpected = append(unexpected, err)
			}
		}(i)
	}
	wg.Wait()
	if len(unexpected) > 0 || len(results) != 8 {
		t.Fatal("concurrent result", len(results), unexpected)
	}
	for _, r := range results {
		if r != results[0] {
			t.Fatal("multiple winners")
		}
	}
	assertNoTemporary(t, root)
	for _, same := range []bool{true, false} {
		t.Run(map[bool]string{true: "same-winner", false: "different-winner"}[same], func(t *testing.T) {
			store, root := testStore(t)
			winner := blue
			if same {
				winner = red
			}
			store.step = func(stage string) error {
				if stage == "synced" {
					return os.WriteFile(imageFile(t, root), winner, 0600)
				}
				return nil
			}
			receipt, err := store.Publish(t.Context(), testAsset, testFP, red)
			if same && err != nil || !same && !errors.Is(err, ErrConflict) {
				t.Fatal("no-replace race", err)
			}
			if !same && receipt != (stillqueue.Image{}) {
				t.Fatal("failed publish returned receipt")
			}
			after, err := os.ReadFile(imageFile(t, root))
			if err != nil || !bytes.Equal(after, winner) {
				t.Fatal("rename overwrote winner")
			}
			assertNoTemporary(t, root)
		})
	}
}
func TestFailureAndCancellationLeaveNoReferencedPartialFile(t *testing.T) {
	raw := testJPEG(t, color.Black, 16, 9)
	for _, stage := range []string{"synced", "renamed"} {
		t.Run(stage, func(t *testing.T) {
			s, root := testStore(t)
			s.step = func(at string) error {
				if at == stage {
					return errors.New("https://synthetic.invalid/?secret=NEVER_LOG")
				}
				return nil
			}
			receipt, err := s.Publish(t.Context(), testAsset, testFP, raw)
			if !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "NEVER_LOG") || receipt != (stillqueue.Image{}) {
				t.Fatal("unsafe receipt/error")
			}
			_, statErr := os.Stat(imageFile(t, root))
			if stage == "synced" && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("partial target appeared")
			}
			if stage == "renamed" && statErr != nil {
				t.Fatal("published orphan disappeared")
			}
			assertNoTemporary(t, root)
			s.step = nil
			if _, err := s.Publish(t.Context(), testAsset, testFP, raw); err != nil {
				t.Fatal("failed retry convergence", err)
			}
		})
	}
	s, root := testStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s.step = func(stage string) error {
		if stage == "synced" {
			cancel()
		}
		return nil
	}
	if _, err := s.Publish(ctx, testAsset, testFP, raw); err != context.Canceled {
		t.Fatal(err)
	}
	assertNoTemporary(t, root)
}
func TestFIFORejectedWithoutBlocking(t *testing.T) {
	s, root := testStore(t)
	if err := os.Mkdir(filepath.Join(root, "ab"), 0700); err != nil {
		t.Fatal(err)
	}
	fifo := imageFile(t, root)
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	path, err := stillqueue.ImagePath(testAsset, testFP)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, _, err := s.Read(t.Context(), path); done <- err }()
	select {
	case err := <-done:
		if err != ErrUnsafe {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		fd, e := unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK, 0)
		if e == nil {
			if e := unix.Close(fd); e != nil {
				t.Error(e)
			}
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("stuck FIFO reader")
		}
		t.Fatal("FIFO open blocked")
	}
}

func TestLeafSymlinkReplacementCannotRedirectIO(t *testing.T) {
	s, root := testStore(t)
	raw := testJPEG(t, color.Black, 16, 9)
	receipt, err := s.Publish(t.Context(), testAsset, testFP, raw)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(realTemp(t), "outside.jpg")
	outsideRaw := testJPEG(t, color.White, 16, 9)
	if err := os.WriteFile(outside, outsideRaw, 0600); err != nil {
		t.Fatal(err)
	}
	file := imageFile(t, root)
	backup := filepath.Join(root, "ab", ".preserved-test-file")
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 40; i++ {
			if err := os.Rename(file, backup); err != nil {
				done <- err
				return
			}
			if err := os.Symlink(outside, file); err != nil {
				if errors.Is(err, os.ErrExist) {
					// The publisher may legitimately win the absent-leaf gap.
					if err := os.Remove(backup); err != nil {
						done <- err
						return
					}
					continue
				}
				done <- err
				return
			}
			if err := os.Remove(file); err != nil {
				done <- err
				return
			}
			if err := os.Rename(backup, file); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 40; i++ {
		body, _, err := s.Read(t.Context(), receipt.RelativePath)
		if err == nil && !bytes.Equal(body, raw) {
			t.Error("read was redirected outside root")
		}
		if err != nil && !errors.Is(err, ErrUnsafe) && !errors.Is(err, ErrNotFound) {
			t.Error(err)
		}
		_, err = s.Publish(t.Context(), testAsset, testFP, raw)
		if err != nil && !errors.Is(err, ErrUnsafe) && !errors.Is(err, ErrNotFound) {
			t.Error(err)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(outside)
	if err != nil || !bytes.Equal(after, outsideRaw) {
		t.Fatal("outside sentinel modified")
	}
}
func TestCloseWaitsForPublicationWithoutReusingDirectoryHandles(t *testing.T) {
	s, _ := testStore(t)
	ready := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	s.step = func(stage string) error {
		if stage == "synced" {
			close(ready)
			<-release
		}
		return nil
	}
	raw := testJPEG(t, color.Black, 16, 9)
	done := make(chan error, 1)
	go func() {
		_, err := s.Publish(t.Context(), testAsset, testFP, raw)
		done <- err
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal("publication failed before barrier", err)
	case <-time.After(5 * time.Second):
		t.Fatal("publication did not reach barrier")
	}
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	select {
	case <-closed:
		t.Fatal("closed directory with an active publisher")
	case <-time.After(20 * time.Millisecond):
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}
func FuzzSplitPath(f *testing.F) {
	f.Add("ab/" + testAsset + "-" + testFP[:16] + ".jpg")
	f.Add("../../outside")
	f.Add("https://synthetic.invalid/private")
	f.Fuzz(func(t *testing.T, path string) {
		shard, leaf, err := splitPath(path)
		if err != nil {
			return
		}
		if shard+"/"+leaf != path || strings.ContainsAny(leaf, "/\\") || len(path) != 60 {
			t.Fatal("unsafe split")
		}
		want, err := stillqueue.ImagePath(leaf[:36], leaf[37:53]+strings.Repeat("0", 48))
		if err != nil || want != path {
			t.Fatal("noncanonical path accepted")
		}
	})
}

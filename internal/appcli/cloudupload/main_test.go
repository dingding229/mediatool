package cloudupload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnedDiagnosticSourceBoundsAndRepeatableHash(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "Owned-Test-5s.mp4")
	content := bytes.Repeat([]byte("self-owned protocol fixture"), 512)
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want := sha256.Sum256(content)
	for i := 0; i < 2; i++ {
		f.Seek(0, io.SeekEnd)
		size, _, hash, err := sourceIdentity(f)
		if err != nil || size != int64(len(content)) || hash != hex.EncodeToString(want[:]) {
			t.Fatal("identity depends on cursor", err)
		}
	}
	for _, item := range []struct {
		name string
		data []byte
	}{{"user-movie.mp4", []byte("x")}, {"Owned-Test-5s.mp4", nil}, {"Owned-Test-5s.mp4", bytes.Repeat([]byte("a"), maxDiagnosticSize+1)}} {
		d := t.TempDir()
		p := filepath.Join(d, item.name)
		os.WriteFile(p, item.data, 0o600)
		bad, _ := os.Open(p)
		_, _, _, err := sourceIdentity(bad)
		bad.Close()
		if err == nil {
			t.Fatal("unbounded/non-diagnostic source accepted")
		}
	}
}
func TestDiagnosticDestinationConfinedToOneRun(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	if !allowedRoot("/115/Mediatool", []string{"/115/Mediatool"}) || allowedRoot("/115", []string{"/115/Mediatool"}) || allowedRoot("/", []string{"/"}) || allowedRoot("/115/Mediatool/../other", []string{"/115"}) {
		t.Fatal("upload root escape")
	}
	good := "/115/Mediatool/.mediatool-upload-check/" + id + "/Owned-Test-5s.mp4"
	if !validDestination("/115/Mediatool", id, good) {
		t.Fatal("correct run rejected")
	}
	for _, bad := range []string{strings.Replace(good, ".mediatool-upload-check", "Movies", 1), good + "/../other", "/115/other/Owned-Test-5s.mp4"} {
		if validDestination("/115/Mediatool", id, bad) {
			t.Fatal("destination escape")
		}
	}
}
func TestDiagnosticJournalIsAtomicPrivateAndRecordsClaim(t *testing.T) {
	p := filepath.Join(t.TempDir(), "upload.json")
	j := journal{RunID: "fixture", State: "starting"}
	if err := saveJournal(p, &j); err != nil {
		t.Fatal(err)
	}
	j.State = "uploading"
	j.SessionID = "private-session"
	if err := saveJournal(p, &j); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var actual journal
	if json.Unmarshal(data, &actual) != nil || actual.SessionID != j.SessionID || actual.UpdatedAt.IsZero() {
		t.Fatal("session not durable")
	}
	stat, _ := os.Stat(p)
	if stat.Mode().Perm() != 0o600 {
		t.Fatal("private journal exposed")
	}
	files, _ := os.ReadDir(filepath.Dir(p))
	if len(files) != 1 {
		t.Fatal("temporary journal leaked")
	}
}
func TestDownloadHashSinkEnforcesWholeOwnedByteBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := hashSink{header: make(http.Header), limit: 4, hash: sha256.New(), cancel: cancel}
	sink.WriteHeader(206)
	if n, err := sink.Write([]byte("test")); n != 4 || err != nil {
		t.Fatal("owned bytes rejected")
	}
	if _, err := sink.Write([]byte("x")); err == nil || ctx.Err() == nil || sink.bytes != 4 {
		t.Fatal("byte budget not enforced")
	}
	if sink.status != 206 {
		t.Fatal("status lost")
	}
}

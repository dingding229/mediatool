// cloud-upload-smoke validates one explicitly authorized, self-owned small
// upload through the production adapter. It does not fabricate download jobs,
// media identity, STRM records, or PT acceptance evidence.
package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

const maxDiagnosticSize = 2 << 20

type journal struct {
	RunID               string    `json:"runId"`
	State               string    `json:"state"`
	ConnectionID        string    `json:"connectionId"`
	ConnectionRevision  int64     `json:"connectionRevision"`
	Destination         string    `json:"destination"`
	DeviceID            string    `json:"deviceId"`
	SessionID           string    `json:"sessionId,omitempty"`
	SourceSize          int64     `json:"sourceSize"`
	SourceSHA1          string    `json:"sourceSha1"`
	SourceSHA256        string    `json:"sourceSha256"`
	RemoteSize          int64     `json:"remoteSize"`
	RemoteHashMatched   bool      `json:"remoteHashMatched"`
	DownloadStatus      int       `json:"downloadStatus"`
	DownloadBytes       int64     `json:"downloadBytes"`
	DownloadHashMatched bool      `json:"downloadHashMatched"`
	FailureClass        string    `json:"failureClass,omitempty"`
	SourcePreserved     bool      `json:"sourcePreserved"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func main() {
	source := flag.String("source", "", "self-owned Owned-Test-5s.mp4, at most 2MiB")
	report := flag.String("report", "", "private, durable journal path (not a temporary container filesystem)")
	confirmed := flag.Bool("write-confirmed", false, "explicitly allow one diagnostic upload within the configured cloud root")
	resume := flag.Bool("resume-confirmed", false, "resume only the persisted diagnostic session; never create a replacement")
	flag.Parse()
	if !*confirmed || *source == "" || *report == "" {
		fmt.Fprintln(os.Stderr, "Explicit write confirmation, owned source and durable journal are required.")
		os.Exit(2)
	}
	if err := run(*source, *report, *resume); err != nil {
		fmt.Fprintln(os.Stderr, "Owned-file upload check did not complete; inspect the private journal. Credentials, URLs and raw provider errors omitted.")
		os.Exit(1)
	}
}

func run(source, report string, resume bool) (result error) {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	size, sha1sum, sha256sum, err := sourceIdentity(file)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil || db == nil {
		return errors.New("database unavailable")
	}
	defer db.Close()
	vault, err := secrets.New(cfg.SecretKey)
	if err != nil {
		return err
	}
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, cfg)
	defer manager.Close()
	connection, err := manager.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return err
	}
	root := connection.String("cloudRoot")
	if root == "" && len(cfg.STRMAllowedRoots) > 0 {
		root = cfg.STRMAllowedRoots[0]
	}
	if !allowedRoot(root, cfg.STRMAllowedRoots) {
		return errors.New("cloud root outside configured roots")
	}
	client, err := manager.CloudByID(ctx, connection.Record.ID)
	if err != nil {
		return err
	}
	j := journal{}
	if data, readErr := os.ReadFile(report); readErr == nil {
		if json.Unmarshal(data, &j) != nil || j.RunID == "" || j.DeviceID == "" || j.ConnectionID != connection.Record.ID || j.ConnectionRevision != connection.Record.Revision || j.SourceSize != size || j.SourceSHA256 != sha256sum || !validDestination(root, j.RunID, j.Destination) {
			return errors.New("journal or source/config changed")
		}
		if j.SessionID == "" {
			return errors.New("unknown start outcome; no blind restart")
		}
		if !resume {
			return errors.New("existing session requires explicit resume")
		}
	} else {
		if !errors.Is(readErr, os.ErrNotExist) || resume {
			return errors.New("journal missing or inaccessible")
		}
		j = journal{RunID: jobs.UUID(), State: "prepared", ConnectionID: connection.Record.ID, ConnectionRevision: connection.Record.Revision, DeviceID: "mediatool-diagnostic-" + jobs.UUID(), SourceSize: size, SourceSHA1: sha1sum, SourceSHA256: sha256sum}
		j.Destination = path.Join(root, ".mediatool-upload-check", j.RunID, "Owned-Test-5s.mp4")
		if err = saveJournal(report, &j); err != nil {
			return err
		}
	}
	defer func() {
		_, _, currentHash, identityErr := sourceIdentity(file)
		j.SourcePreserved = identityErr == nil && currentHash == j.SourceSHA256
		if result == nil && !j.SourcePreserved {
			result = errors.New("local source changed")
		}
		if result != nil {
			j.FailureClass = failureClass(result)
			if j.State == "starting" && j.SessionID == "" {
				j.State = "uncertain"
			} else {
				j.State = "needs_reconciliation"
			}
		}
		if saveErr := saveJournal(report, &j); saveErr != nil {
			result = errors.New("failed to preserve diagnostic journal")
		}
		// The session ID/path remain private. Public output contains only facts.
		json.NewEncoder(os.Stdout).Encode(map[string]any{"state": j.State, "sessionPersisted": j.SessionID != "", "sourceSize": j.SourceSize, "remoteSize": j.RemoteSize, "remoteHashMatched": j.RemoteHashMatched, "downloadStatus": j.DownloadStatus, "downloadBytes": j.DownloadBytes, "downloadHashMatched": j.DownloadHashMatched, "failureClass": j.FailureClass, "businessDownloadRecordsCreated": false, "mediaIdentityAssigned": false, "sourcePreserved": j.SourcePreserved})
	}()
	existing, lookupErr := client.Find(ctx, j.Destination)
	if lookupErr == nil {
		if j.SessionID == "" {
			return errors.New("destination already exists; not overwriting")
		}
		return verifyUploaded(ctx, client, file, report, &j, existing)
	}
	if !errors.Is(lookupErr, storage.ErrNotFound) {
		return lookupErr
	}
	if err = client.EnsureFolder(ctx, path.Dir(j.Destination), root); err != nil {
		return err
	}
	if j.SessionID != "" {
		if err = client.ResumeUpload(ctx, j.SessionID); err != nil {
			return err
		}
	} else {
		j.State = "starting"
		if err = saveJournal(report, &j); err != nil {
			return err
		}
	}
	remote, err := client.UploadFile(ctx, clouddrive2.UploadRequest{File: file, Destination: j.Destination, DeviceID: j.DeviceID, SessionID: j.SessionID, ExpectedSHA1: sha1sum, OnStart: func(id string) error {
		j.SessionID = id
		j.State = "uploading"
		return saveJournal(report, &j)
	}, OnProgress: func(_ int64, state string) error {
		j.State = "upload_" + state
		return saveJournal(report, &j)
	}})
	if err != nil {
		return err
	}
	return verifyUploaded(ctx, client, file, report, &j, remote)
}

func sourceIdentity(file *os.File) (int64, string, string, error) {
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > maxDiagnosticSize || filepath.Base(file.Name()) != "Owned-Test-5s.mp4" {
		return 0, "", "", errors.New("only the self-owned small diagnostic source is allowed")
	}
	sha1hash, sha256hash := sha1.New(), sha256.New()
	if _, err = io.Copy(io.MultiWriter(sha1hash, sha256hash), io.NewSectionReader(file, 0, stat.Size())); err != nil {
		return 0, "", "", err
	}
	return stat.Size(), hex.EncodeToString(sha1hash.Sum(nil)), hex.EncodeToString(sha256hash.Sum(nil)), nil
}
func allowedRoot(root string, roots []string) bool {
	if root == "" || root == "/" || path.Clean(root) != root || !strings.HasPrefix(root, "/") || strings.ContainsAny(root, "\r\n\x00") {
		return false
	}
	for _, allowed := range roots {
		if root == allowed || strings.HasPrefix(root, allowed+"/") {
			return true
		}
	}
	return false
}
func validDestination(root, runID, destination string) bool {
	// UUID-only run directories, no user-selected destination, no path traversal.
	if len(runID) != 36 {
		return false
	}
	for i, r := range runID {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return destination == path.Join(root, ".mediatool-upload-check", runID, "Owned-Test-5s.mp4")
}
func saveJournal(file string, j *journal) error {
	j.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(file), ".upload-journal-")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, file); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(file))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func failureClass(err error) string {
	for _, item := range []struct {
		err  error
		name string
	}{{storage.ErrUnsupported, "rpc_unsupported"}, {storage.ErrForbidden, "provider_permission_denied"}, {storage.ErrUnauthenticated, "provider_auth_denied"}, {storage.ErrUnavailable, "provider_unavailable_or_interrupted"}, {storage.ErrInvalidResponse, "provider_response_invalid"}} {
		if errors.Is(err, item.err) {
			return item.name
		}
	}
	return "verification_or_control_failed"
}

type hashSink struct {
	header       http.Header
	status       int
	bytes, limit int64
	hash         hash.Hash
	cancel       context.CancelFunc
}

func (s *hashSink) Header() http.Header { return s.header }
func (s *hashSink) WriteHeader(status int) {
	if s.status == 0 {
		s.status = status
	}
}
func (s *hashSink) Write(data []byte) (int, error) {
	if s.status == 0 {
		s.status = 200
	}
	if s.bytes+int64(len(data)) > s.limit {
		s.cancel()
		return 0, errors.New("diagnostic byte budget exceeded")
	}
	n, err := s.hash.Write(data)
	s.bytes += int64(n)
	return n, err
}
func verifyUploaded(ctx context.Context, client *clouddrive2.Adapter, file *os.File, report string, j *journal, remote storage.File) error {
	j.RemoteSize = remote.Size
	j.RemoteHashMatched = strings.EqualFold(remote.Hashes[uint32(2)], j.SourceSHA1)
	if remote.IsDirectory || remote.Size != j.SourceSize || remote.Hashes[uint32(2)] != "" && !j.RemoteHashMatched {
		return errors.New("remote size or type mismatch")
	}
	// Even if the provider omits file hashes, verify ALL bytes of this bounded
	// self-owned file via the same public-target/TLS checked download engine.
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	engine := playback.New(client, playback.Options{Policy: playback.Policy{BlockedHosts: []string{"localhost", "server", "clouddrive2", "emby", "185.99.135.205"}}, ProxyFallback: true, ForceProxy: true})
	req, _ := http.NewRequestWithContext(bounded, "GET", "http://diagnostic.invalid/media", nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", j.SourceSize-1))
	req.Header.Set("User-Agent", "MediaTool-OwnedUploadCheck/1.0")
	sink := &hashSink{header: make(http.Header), limit: j.SourceSize, hash: sha256.New(), cancel: cancel}
	engine.Serve(sink, req, storage.FileRef{Path: j.Destination, ExpectedSize: j.SourceSize})
	j.DownloadStatus, j.DownloadBytes = sink.status, sink.bytes
	j.DownloadHashMatched = hex.EncodeToString(sink.hash.Sum(nil)) == j.SourceSHA256
	_, _, current, err := sourceIdentity(file)
	if err != nil || current != j.SourceSHA256 {
		return errors.New("local source changed")
	}
	if (sink.status != 200 && sink.status != 206) || sink.bytes != j.SourceSize || !j.DownloadHashMatched {
		return errors.New("remote bytes do not match source")
	}
	j.State = "verified"
	j.FailureClass = ""
	return saveJournal(report, j)
}

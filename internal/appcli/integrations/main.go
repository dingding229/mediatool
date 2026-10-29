// integration-smoke reads saved encrypted provider connections without creating
// administrator sessions, resetting passwords or printing credentials/URLs.
package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"media-tool/internal/appcli/diagnosticpolicy"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/mediaext"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

var errListingBudget = errors.New("diagnostic listing budget reached")

type limitedSink struct {
	header        http.Header
	status, bytes int
	hash          []byte
	cancel        context.CancelFunc
}

func (s *limitedSink) Header() http.Header    { return s.header }
func (s *limitedSink) WriteHeader(status int) { s.status = status }
func (s *limitedSink) Write(data []byte) (int, error) {
	if s.status == 0 {
		s.status = 200
	}
	n := min(len(data), 1024-s.bytes)
	if n > 0 {
		s.bytes += n
		digest := sha256.Sum256(data[:n])
		s.hash = digest[:]
	}
	if s.bytes >= 1024 {
		s.cancel()
		return n, io.EOF
	}
	return n, nil
}
func Main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Integration diagnostic failed; provider secrets and raw errors omitted.")
		os.Exit(1)
	}
}
func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := database.Open(ctx, c.DatabaseURL)
	if err != nil || db == nil {
		return errors.New("database required")
	}
	defer db.Close()
	vault, err := secrets.New(c.SecretKey)
	if err != nil {
		return err
	}
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, c)
	defer manager.Close()
	report := map[string]any{"checkedAt": time.Now().UTC().Format(time.RFC3339), "passwordResetPerformed": false, "strmGenerated": false}
	cloud, err := manager.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return err
	}
	client, err := manager.CloudByID(ctx, cloud.Record.ID)
	if err != nil {
		return err
	}
	root := cloud.String("cloudRoot")
	probe, err := client.Probe(ctx, root)
	if err != nil {
		report["cloudReadAuthorized"] = false
		if checkedErr := json.NewEncoder(os.Stdout).Encode(report); checkedErr != nil {
			return checkedErr
		}
		return errors.New("cloud probe failed")
	}
	report["cloudReadAuthorized"] = probe.PathReadable
	report["cloudLoggedIn"] = probe.LoggedIn
	report["cloudRootReadable"] = probe.PathReadable
	directories := []string{root}
	visited, entries := 0, 0
	var selected *storage.File
	for len(directories) > 0 && visited < 12 && entries < 600 && selected == nil {
		directory := directories[0]
		directories = directories[1:]
		visited++
		listErr := client.Iterate(ctx, directory, func(file storage.File) error {
			entries++
			if file.IsDirectory {
				if path.Dir(file.Path) == directory && strings.HasPrefix(file.Path, strings.TrimRight(root, "/")+"/") && len(directories) < 20 {
					directories = append(directories, file.Path)
				}
			} else {
				if file.Size > 0 && mediaext.IsVideo(file.Path) {
					copy := file
					selected = &copy
					return errListingBudget
				}
			}
			if entries >= 600 {
				return errListingBudget
			}
			return nil
		})
		if listErr != nil && !errors.Is(listErr, errListingBudget) {
			report["cloudListingCompleted"] = false
			break
		}
	}
	report["directoriesChecked"] = visited
	report["entriesChecked"] = entries
	report["videoFound"] = selected != nil
	if selected != nil {
		// Path/title remain user data; only a digest and size leave this diagnostic.
		digest := sha256.Sum256([]byte(selected.Path))
		report["testFilePathDigest"] = hex.EncodeToString(digest[:])
		report["testFileBytes"] = selected.Size
		ref := storage.FileRef{Path: selected.Path, UserAgent: "MediaTool-IntegrationCheck"}
		if capabilities, err := client.DiagnoseLink(ctx, selected.Path); err == nil {
			report["cloudLinkCapabilities"] = capabilities
		} else {
			report["cloudLinkCapabilitiesReadable"] = false
		}

		engine := playback.New(integrations.DynamicStorage{Manager: manager}, playback.Options{Policy: diagnosticpolicy.New(c.PublicURL, c.MemberPublicURL, cloud.String("endpoint"))})
		inspection, inspectErr := engine.Inspect(ctx, ref, "MediaTool-IntegrationCheck")
		report["linkPolicyValidated"] = inspectErr == nil && inspection.TargetValidated
		if inspectErr != nil {
			reason := "provider_response_or_access_failed"
			switch {
			case errors.Is(inspectErr, storage.ErrForbidden):
				reason = "provider_permission_denied"
			case errors.Is(inspectErr, storage.ErrUnauthenticated):
				reason = "provider_token_denied"
			case errors.Is(inspectErr, storage.ErrNoDirectURL):
				reason = "provider_returned_no_direct_url"
			case errors.Is(inspectErr, playback.ErrUnsafeTarget):
				reason = "target_policy_rejected"
			case errors.Is(inspectErr, storage.ErrInvalidResponse):
				reason = "provider_response_invalid"
			}
			report["linkFailureClass"] = reason
			if link, resolveErr := client.Resolve(ctx, ref); resolveErr == nil {
				parsed, parseErr := url.Parse(link.URL)
				if parseErr == nil {
					report["returnedLinkScheme"] = parsed.Scheme
					policy := diagnosticpolicy.New(c.PublicURL, c.MemberPublicURL, cloud.String("endpoint"))
					_, syntaxErr := policy.Syntax(link.URL)
					report["returnedLinkSyntaxAllowed"] = syntaxErr == nil
					report["returnedLinkTargetAllowed"] = policy.Check(ctx, link.URL) == nil
				}
			}
		}
		if inspectErr == nil {
			requestCtx, stop := context.WithTimeout(ctx, 20*time.Second)
			req, _ := http.NewRequestWithContext(requestCtx, "GET", "http://diagnostic.invalid/media", nil)
			req.Header.Set("Range", "bytes=0-1023")
			req.Header.Set("User-Agent", "MediaTool-IntegrationCheck")
			sink := &limitedSink{header: make(http.Header), cancel: stop}
			engine.Serve(sink, req, ref)
			stop()
			report["mediaHTTPStatus"] = sink.status
			report["sampleBytes"] = sink.bytes
			report["mediaBytesReadable"] = (sink.status == 200 || sink.status == 206) && sink.bytes > 0
		}
	}
	if checkedErr := json.NewEncoder(os.Stdout).Encode(report); checkedErr != nil {
		return checkedErr
	}
	return nil
}

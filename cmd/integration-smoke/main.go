// integration-smoke reads saved encrypted provider connections without creating
// administrator sessions, resetting passwords or printing credentials/URLs.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
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
func main() {
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
	report := map[string]any{"checkedAt": time.Now().UTC().Format(time.RFC3339), "passwordResetPerformed": false, "strmGenerated": false, "embyClientPlaybackVerified": false}
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
		json.NewEncoder(os.Stdout).Encode(report)
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
				ext := strings.ToLower(path.Ext(file.Path))
				if file.Size > 0 && (ext == ".mp4" || ext == ".mkv" || ext == ".avi" || ext == ".mov" || ext == ".m4v" || ext == ".ts") {
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

		engine := playback.New(integrations.DynamicStorage{Manager: manager}, playback.Options{Policy: playback.Policy{BlockedHosts: []string{"localhost", "server", "clouddrive2", "emby", "185.99.135.205"}}, ProxyFallback: true, ForceProxy: true})
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
					policy := playback.Policy{BlockedHosts: []string{"localhost", "server", "clouddrive2", "emby", "185.99.135.205"}}
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
	emby, err := manager.Repository.Default(ctx, "emby")
	if err != nil {
		return err
	}
	report["embyRefreshKeyPresent"] = emby.Values["apiKey"] != ""
	report["configuredSTRMPathPrefix"] = emby.String("strmPathPrefix")
	report["gatewayEnabled"] = emby.Record.Settings["gatewayEnabled"] == true
	httpClient := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(endpoint string, target any) (int, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(emby.String("endpoint"), "/")+endpoint, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("X-Emby-Token", emby.Values["apiKey"])
		res, err := httpClient.Do(req)
		if err != nil {
			return 0, err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return res.StatusCode, errors.New("provider rejected")
		}
		return res.StatusCode, json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(target)
	}
	var users []struct {
		ID string `json:"Id"`
	}
	status, authErr := get("/Users", &users)
	report["embyKeyAuthorized"] = authErr == nil
	report["embyAuthenticatedStatus"] = status
	var libraries []struct {
		Name      string   `json:"Name"`
		Locations []string `json:"Locations"`
	}
	_, libraryErr := get("/Library/VirtualFolders", &libraries)
	report["embyLibrariesReadable"] = libraryErr == nil
	locations := []string{}
	for _, library := range libraries {
		locations = append(locations, library.Locations...)
	}
	report["embyLibraryLocations"] = locations
	json.NewEncoder(os.Stdout).Encode(report)
	return nil
}

// playback-smoke verifies actual file grants and library visibility without
// knowing administrator passwords or substituting service keys for user tokens.
package playback

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
	"media-tool/internal/modules/playbackgrants"
	"media-tool/internal/modules/storage"
	deferutil "media-tool/internal/platform/cleanup"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
	"net/http"
	"net/url"
	"os"

	"strings"
	"time"
)

func Main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Playback diagnostic failed; keys, signed URLs and private exceptions omitted.")
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil || db == nil {
		return errors.New("database required")
	}
	defer db.Close()
	vault, err := secrets.New(cfg.SecretKey)
	if err != nil {
		return err
	}
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, cfg)
	defer manager.Close()
	var assetID, output string
	if err = db.QueryRow(ctx, `SELECT asset_id::text,output_path FROM strm_entries ORDER BY updated_at DESC LIMIT 1`).Scan(&assetID, &output); err != nil {
		return err
	}
	service := playbackgrants.Service{DB: db, Vault: vault, PublicURL: cfg.PublicURL}
	link, err := service.ExistingURL(ctx, assetID)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(cfg.STRMOutputDir)
	if err != nil {
		return err
	}
	defer deferutil.Close(root, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	file, err := root.Open(output)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(file, 8193))
	deferutil.Close(file, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	if err != nil || len(raw) > 8192 {
		return errors.New("invalid artifact")
	}
	saved := strings.TrimSpace(string(raw))
	u, err := url.Parse(saved)
	if err != nil {
		return err
	}
	reference, err := service.VerifyManagedURL(ctx, saved)
	if err != nil {
		return err
	}
	report := map[string]any{"assetId": assetID, "fileGrantValidated": reference.AssetID == assetID, "savedSTRMHasFixedAssetId": u.Query().Get("assetId") == assetID, "savedSTRMOriginMatchesCurrent": u.Scheme+"://"+u.Host == cfg.PublicURL, "realPlayerDecoded": false}
	noRedirect := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(method, target string) (*http.Response, error) {
		r, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Set("User-Agent", "MediaTool-PlaybackCheck")
		r.Header.Set("Range", "bytes=0-1023")
		return noRedirect.Do(r)
	}
	providerLink, providerErr := (integrations.DynamicStorage{Manager: manager}).Resolve(ctx, storage.FileRef{AssetID: assetID, Path: reference.Path})
	if providerErr == nil {
		report["providerRequiresUserAgent"] = providerLink.UserAgent != ""
		report["providerRequiredHeaderCount"] = len(providerLink.Headers)
		report["forceProxyConfigured"] = cfg.STRMForceProxy
		if len(providerLink.Headers) == 0 && !cfg.STRMForceProxy {
			compatible, _ := http.NewRequestWithContext(ctx, "HEAD", link, nil)
			agent := providerLink.UserAgent
			if agent == "" {
				agent = "MediaTool-PlaybackCheck"
			}
			compatible.Header.Set("User-Agent", agent)
			compatible.Header.Set("Range", "bytes=0-1023")
			compatibleResponse, err := noRedirect.Do(compatible)
			if err == nil {
				report["compatibleUserAgentStatus"] = compatibleResponse.StatusCode
				report["compatibleRequestRedirected"] = compatibleResponse.StatusCode == 302
				report["compatibleRedirectTargetValidated"] = compatibleResponse.StatusCode == 302 && (playback.Policy{BlockedHosts: []string{"185.99.135.205", "localhost", "server", "emby", "clouddrive2"}}).Check(ctx, compatibleResponse.Header.Get("Location")) == nil
				deferutil.Close(compatibleResponse.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
			}
		}
	}
	head, err := request("HEAD", link)
	if err != nil {
		return err
	}
	if head.StatusCode != 302 {
		deferutil.Close(head.Body, "strict resolver rejection")
		return errors.New("stable STRM must return 302; relay forbidden")
	}
	report["fixedSTRMHeadStatus"] = head.StatusCode
	report["headBodyBytes"] = 0
	deferutil.Close(head.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	response, err := request("GET", link)
	if err != nil {
		return err
	}
	report["fixedSTRMGetStatus"] = response.StatusCode
	location := response.Header.Get("Location")
	report["deliveryMode"] = response.Header.Get("X-Media-Delivery")
	var media *http.Response
	if response.StatusCode == 302 {
		deferutil.Close(response.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
		policy := playback.Policy{BlockedHosts: []string{"185.99.135.205", "localhost", "server", "emby", "clouddrive2"}}
		validErr := policy.Check(ctx, location)
		report["redirectTargetValidated"] = validErr == nil
		if validErr == nil {
			media, err = request("GET", location)
		}
	} else {
		deferutil.Close(response.Body, "strict resolver rejection")
		err = errors.New("stable STRM must return 302; relay forbidden")
	}
	if err == nil && media != nil {
		sample, readErr := io.ReadAll(io.LimitReader(media.Body, 1024))
		deferutil.Close(media.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
		digest := sha256.Sum256(sample)
		report["mediaStatus"] = media.StatusCode
		report["sampleBytes"] = len(sample)
		report["sampleSha256"] = hex.EncodeToString(digest[:])
		report["rangeResponse"] = media.StatusCode == 206
		report["mediaBytesVerified"] = readErr == nil && media.StatusCode == 206 && len(sample) > 0
	} else {
		report["mediaBytesVerified"] = false
	}
	bad := *u
	q := bad.Query()
	q.Set("apiKey", "invalid-diagnostic-key")
	bad.RawQuery = q.Encode()
	denied, err := request("GET", bad.String())
	if err == nil {
		report["invalidFileKeyRejected"] = denied.StatusCode == 401 && denied.Header.Get("Location") == ""
		deferutil.Close(denied.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

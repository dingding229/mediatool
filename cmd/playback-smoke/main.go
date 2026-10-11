// playback-smoke verifies actual file grants and library visibility without
// knowing administrator passwords or substituting service keys for user tokens.
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
	"media-tool/internal/modules/playbackgrants"
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

func main() {
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
	defer root.Close()
	file, err := root.Open(output)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(file, 8193))
	file.Close()
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
	report := map[string]any{"assetId": assetID, "fileGrantValidated": reference.AssetID == assetID, "savedSTRMHasFixedAssetId": u.Query().Get("assetId") == assetID, "savedSTRMOriginMatchesCurrent": u.Scheme+"://"+u.Host == cfg.PublicURL, "embyUserAuthorizedGatewayVerified": false, "realPlayerDecoded": false}
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
				compatibleResponse.Body.Close()
			}
		}
	}
	head, err := request("HEAD", link)
	if err != nil {
		return err
	}
	report["fixedSTRMHeadStatus"] = head.StatusCode
	report["headBodyBytes"] = 0
	head.Body.Close()
	response, err := request("GET", link)
	if err != nil {
		return err
	}
	report["fixedSTRMGetStatus"] = response.StatusCode
	location := response.Header.Get("Location")
	report["deliveryMode"] = response.Header.Get("X-Media-Delivery")
	var media *http.Response
	if response.StatusCode == 302 {
		response.Body.Close()
		policy := playback.Policy{BlockedHosts: []string{"185.99.135.205", "localhost", "server", "emby", "clouddrive2"}}
		validErr := policy.Check(ctx, location)
		report["redirectTargetValidated"] = validErr == nil
		if validErr == nil {
			media, err = request("GET", location)
		}
	} else {
		media = response
	}
	if err == nil && media != nil {
		sample, readErr := io.ReadAll(io.LimitReader(media.Body, 1024))
		media.Body.Close()
		digest := sha256.Sum256(sample)
		report["mediaStatus"] = media.StatusCode
		report["sampleBytes"] = len(sample)
		report["sampleSha256"] = hex.EncodeToString(digest[:])
		report["rangeResponse"] = media.StatusCode == 206
		report["mediaBytesVerified"] = readErr == nil && (media.StatusCode == 200 || media.StatusCode == 206) && len(sample) > 0
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
		denied.Body.Close()
	}
	emby, err := manager.Repository.Default(ctx, "emby")
	if err != nil {
		return err
	}
	expected := path.Join(emby.String("strmPathPrefix"), output)
	foundID := ""
	for start := 0; start < 1000 && foundID == ""; start += 200 {
		query := url.Values{"Recursive": {"true"}, "Fields": {"Path,MediaSources"}, "Limit": {"200"}, "StartIndex": {fmt.Sprint(start)}}
		r, _ := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(emby.String("endpoint"), "/")+"/Items?"+query.Encode(), nil)
		r.Header.Set("X-Emby-Token", emby.Values["apiKey"])
		res, err := noRedirect.Do(r)
		if err != nil {
			return err
		}
		var page struct {
			Items []struct {
				ID      string `json:"Id"`
				Path    string `json:"Path"`
				Sources []struct {
					Path string `json:"Path"`
				} `json:"MediaSources"`
			}
			Total int `json:"TotalRecordCount"`
		}
		if res.StatusCode != 200 {
			res.Body.Close()
			return errors.New("Emby metadata refused")
		}
		err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&page)
		res.Body.Close()
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			if item.Path == expected {
				foundID = item.ID
			}
			for _, source := range item.Sources {
				if source.Path == expected {
					foundID = item.ID
				}
			}
		}
		if start+len(page.Items) >= page.Total || len(page.Items) == 0 {
			break
		}
	}
	report["embyManagedSTRMVisible"] = foundID != ""
	if foundID != "" {
		report["embyItemId"] = foundID
		r, _ := http.NewRequestWithContext(ctx, "GET", cfg.EmbyPublicURL+"/Videos/"+url.PathEscape(foundID)+"/stream?Static=true", nil)
		r.Header.Set("X-Emby-Token", "invalid-diagnostic-user-token")
		denied, err := noRedirect.Do(r)
		if err == nil {
			report["invalidEmbyUserRejected"] = denied.StatusCode == 401 || denied.StatusCode == 403
			report["invalidEmbyUserStatus"] = denied.StatusCode
			denied.Body.Close()
		}
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

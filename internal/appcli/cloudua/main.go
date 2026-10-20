// Explicit reversible maintenance only; aligns the provider's signed-download
// UA with a native player without weakening redirect or authorization checks.
package cloudua

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	deferutil "media-tool/internal/platform/cleanup"
	"net/http"
	"os"
	"strings"
	"time"

	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/playback"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func Main() {
	apply := flag.Bool("apply", false, "try only customUserAgent; restore on failed direct-link verification")
	agent := flag.String("user-agent", "", "native player User-Agent to align")
	flag.Parse()
	if !*apply || *agent == "" {
		fmt.Fprintln(os.Stderr, "Explicit --apply and --user-agent required; no settings changed.")
		os.Exit(2)
	}
	if err := run(*agent); err != nil {
		fmt.Fprintln(os.Stderr, "UA alignment was not confirmed; raw provider details and credentials omitted.")
		os.Exit(1)
	}
}
func run(agent string) error {
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
	connection, err := manager.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return err
	}
	adapter, err := manager.CloudByID(ctx, connection.Record.ID)
	if err != nil {
		return err
	}
	var ref storage.FileRef
	if err = db.QueryRow(ctx, `SELECT a.id::text,f.full_path FROM media_assets a JOIN cloud_files f ON f.id=a.cloud_file_id JOIN strm_entries s ON s.asset_id=a.id WHERE f.state='present' AND f.storage_id=$1 ORDER BY s.updated_at DESC LIMIT 1`, connection.Record.ID).Scan(&ref.AssetID, &ref.Path); err != nil {
		return err
	}
	policy := playback.Policy{BlockedHosts: []string{"185.99.135.205", "localhost", "server", "emby", "clouddrive2"}}
	client := http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil, DialContext: policy.DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DisableCompression: true}}
	verified := false
	changed, err := adapter.TryDownloadUserAgent(ctx, ref.Path, agent, func(check context.Context) error {
		link, err := adapter.Resolve(check, ref)
		if err != nil {
			return err
		}
		if link.UserAgent != agent || len(link.Headers) != 0 || !strings.HasPrefix(link.URL, "https://") || policy.Check(check, link.URL) != nil {
			return errors.New("provider did not issue matching native UA link")
		}
		request, err := http.NewRequestWithContext(check, "GET", link.URL, nil)
		if err != nil {
			return err
		}
		request.Header.Set("User-Agent", agent)
		request.Header.Set("Range", "bytes=0-1023")
		res, err := client.Do(request)
		if err != nil {
			return errors.New("native CDN bytes unavailable")
		}
		defer deferutil.Close(res.Body, "Process teardown only; business results are already checked and OS releases remaining descriptors on exit")
		sample, err := io.ReadAll(io.LimitReader(res.Body, 1024))
		if err != nil || res.StatusCode != 206 || len(sample) != 1024 || !strings.HasPrefix(res.Header.Get("Content-Range"), "bytes 0-1023/") {
			return errors.New("native UA not accepted")
		}
		verified = true
		return nil
	})
	if err != nil && changed {
		return err
	}
	if err == nil && changed {
		if _, err = db.Exec(ctx, `UPDATE integrations SET revision=revision+1,updated_at=now() WHERE id=$1`, connection.Record.ID); err != nil {
			return err
		}
		if _, err = db.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('authorized-operator','clouddrive.download_user_agent',jsonb_build_object('integrationId',$1::text))`, connection.Record.ID); err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"nativeUserAgentAligned": verified, "changed": changed, "otherCloudSettingsPreserved": true, "failedVerificationRestored": err != nil && !changed, "accountsModified": false})
}

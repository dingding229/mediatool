// Read-only, authorized-project capability check. No event is stored, no scan
// is created, and provider fields/paths/credentials are never exported.
package cloudevents

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"media-tool/internal/adapters/clouddrive2"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/storage"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func Main() {
	duration := flag.Duration("duration", 5*time.Second, "read-only observation window, 1s..30s")
	flag.Parse()
	if *duration < time.Second || *duration > 30*time.Second {
		fmt.Fprintln(os.Stderr, "duration must be 1s..30s")
		os.Exit(2)
	}
	if err := run(*duration); err != nil {
		fmt.Fprintln(os.Stderr, "CloudDrive2 change capability could not be checked; raw details omitted")
		os.Exit(1)
	}
}
func run(duration time.Duration) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration+5*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil || db == nil {
		return errors.New("configured database required")
	}
	defer db.Close()
	vault, err := secrets.New(cfg.SecretKey)
	if err != nil {
		return err
	}
	manager := integrations.NewManager(integrations.Repository{DB: db, Vault: vault}, cfg)
	defer manager.Close()
	credentials, err := manager.Repository.Default(ctx, "clouddrive2")
	if err != nil {
		return err
	}
	client, err := manager.CloudByID(ctx, credentials.Record.ID)
	if err != nil {
		return err
	}
	scope := credentials.String("cloudRoot")
	if scope == "" && len(cfg.STRMAllowedRoots) > 0 {
		scope = cfg.STRMAllowedRoots[0]
	}
	if scope == "" {
		return errors.New("configured scope required")
	}
	report := struct {
		CheckedAt                      time.Time `json:"checkedAt"`
		ReadOnly                       bool      `json:"readOnly"`
		WindowSeconds                  int       `json:"windowSeconds"`
		StreamMessageObserved          bool      `json:"streamMessageObserved"`
		ScopedHints                    int       `json:"scopedHints"`
		Outcome                        string    `json:"outcome"`
		RawProviderDataExported        bool      `json:"rawProviderDataExported"`
		RealFileChangeDeliveryVerified bool      `json:"realFileChangeDeliveryVerified"`
	}{CheckedAt: time.Now().UTC(), ReadOnly: true, WindowSeconds: int(duration / time.Second)}
	observe, stop := context.WithTimeout(ctx, duration)
	defer stop()
	err = client.ObserveChanges(observe, func() error { report.StreamMessageObserved = true; return nil }, func(h clouddrive2.FileChange) error {
		for _, p := range []string{h.Path, h.NewPath} {
			if p == scope || strings.HasPrefix(p, strings.TrimRight(scope, "/")+"/") {
				report.ScopedHints++
				break
			}
		}
		return nil
	})
	switch {
	case errors.Is(err, storage.ErrUnsupported):
		report.Outcome = "unsupported"
	case report.StreamMessageObserved && (err == nil || errors.Is(err, context.DeadlineExceeded)):
		report.Outcome = "rpc_window_observed"
	case errors.Is(err, storage.ErrUnauthenticated):
		report.Outcome = "authentication_failed"
	case errors.Is(err, storage.ErrForbidden):
		report.Outcome = "permission_denied"
	default:
		report.Outcome = "unavailable"
	}
	// A bounded idle stream is not evidence of an actual filesystem mutation.
	return json.NewEncoder(os.Stdout).Encode(report)
}

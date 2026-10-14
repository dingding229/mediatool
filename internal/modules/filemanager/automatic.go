package filemanager

import (
	"context"
	"crypto/sha256"
	"fmt"
	"media-tool/internal/modules/integrations"
	"os"
	"path/filepath"
	"strings"
)

// Local roots are discovered from the application's accessible data directories,
// not created through a UI binding. No host-root or pseudo-filesystem mount.
func RegisterLocal(ctx context.Context, manager *integrations.Manager, paths []string) error {
	for _, raw := range paths {
		p, err := filepath.Abs(strings.TrimSpace(raw))
		if err != nil || !safeLocalRoot(p) {
			continue
		}
		st, err := os.Lstat(p)
		if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		sum := sha256.Sum256([]byte("media-tool:auto-local:" + p))
		id := fmt.Sprintf("%x-%x-%x-%x-%x", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
		encrypted, sealErr := manager.Repository.Vault.Seal(id, map[string]string{})
		if sealErr != nil {
			return sealErr
		}
		_, err = manager.Repository.DB.Exec(ctx, `INSERT INTO integrations(id,kind,name,enabled,settings,credentials_ciphertext)VALUES($1,'local',$2,true,jsonb_build_object('rootPath',$3::text),$4) ON CONFLICT(id) DO NOTHING`, id, "本地目录 · "+p, p, encrypted)
		if err != nil {
			return err
		}
	}
	return nil
}
func privateLocalPath(p string) bool {
	for _, name := range strings.Split(filepath.ToSlash(p), "/") {
		lower := strings.ToLower(name)
		if lower == "password" || strings.HasPrefix(lower, ".password") || lower == "logs" || lower == "postgres" || lower == "redis" || lower == "private" || lower == "backups" || lower == ".env" || strings.HasPrefix(lower, ".env.") || lower == "docker-compose.yml" || lower == "docker.sock" || lower == "pgdata" || lower == "redis-data" || lower == "postgresql" || lower == ".ssh" || lower == ".git" || lower == "secrets" || lower == "diagnostics" {
			return true
		}
	}
	return false
}

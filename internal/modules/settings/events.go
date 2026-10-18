package settings

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/platform/outbox"
	"reflect"
	"slices"
	"strconv"
)

// Only key names from this list are exported, never values, network/security
// settings, path templates, account identities or arbitrary keys.
func changedSettings(a, b Value) []string {
	fields := []string{}
	if a.CleanupAfterImport != b.CleanupAfterImport {
		fields = append(fields, "cleanupAfterImport")
	}
	if a.SearchCacheSeconds != b.SearchCacheSeconds {
		fields = append(fields, "searchCacheSeconds")
	}
	if a.TMDBCacheSeconds != b.TMDBCacheSeconds {
		fields = append(fields, "tmdbCacheSeconds")
	}
	if a.SubscriptionCron != b.SubscriptionCron {
		fields = append(fields, "subscriptionCron")
	}
	if !reflect.DeepEqual(a.Scraping, b.Scraping) {
		fields = append(fields, "scraping")
	}
	slices.Sort(fields)
	return fields
}
func emitSettings(ctx context.Context, tx pgx.Tx, area string, revision int64, fields []string) error {
	return outbox.EmitVersionTx(ctx, tx, "settings.changed", area, strconv.FormatInt(revision, 10), contracts.SettingsChanged{Area: area, Revision: revision, Changed: fields})
}
func nativeChanges(before []byte, in NativeManagement) []string {
	var old NativeManagement
	_ = json.Unmarshal(before, &old)
	fields := []string{}
	if old.ServerName != in.ServerName {
		fields = append(fields, "serverName")
	}
	if old.Enabled != in.Enabled {
		fields = append(fields, "enabled")
	}
	return fields
}

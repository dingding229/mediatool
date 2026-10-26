package library

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/nativetmdb"
	pluginv2 "media-tool/internal/modules/plugins/v2"
)

// Refresh keeps the selected plugin extension contract, but the builtin fallback
// only queues native metadata. No external server identity/path/HTTP is involved.
func (e Engine) Refresh(ctx context.Context, j jobs.Job) error {
	var p struct {
		AssetID string `json:"assetId"`
	}
	if json.Unmarshal(j.Payload, &p) != nil || p.AssetID == "" {
		return errors.New("invalid native refresh")
	}
	if e.Connections == nil {
		return errors.New("native metadata connection unavailable")
	}
	store := pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}
	request := contracts.EffectRequest{RequestID: j.ID, ObjectID: p.AssetID, Kind: "asset"}
	receipt, err := store.ExternalEffect(ctx, "mediaserver.refresh", request)
	if err != nil || receipt.State == "unknown" {
		return jobs.ErrOperatorRequired
	}
	if receipt.State != "not_started" {
		return nil
	}
	language, err := (nativetmdb.Engine{DB: e.DB, Connections: e.Connections}).Language(ctx)
	if err != nil {
		return err
	}
	return store.BuiltinEffect(ctx, "mediaserver.refresh", request, func(ctx context.Context) error {
		return (nativetmdb.Repository{DB: e.DB}).EnqueueAsset(ctx, p.AssetID, language)
	})
}

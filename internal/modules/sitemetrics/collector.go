package sitemetrics

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/trace"
	"time"
)

var ErrCollection = errors.New("site collection failed")
var ErrBusy = errors.New("site metrics already refreshing")

func (e Engine) Collect(ctx context.Context, id string) error {
	// Use a dedicated lock connection so a pool full of concurrent collectors
	// cannot deadlock while repository reads/writes wait for another pooled connection.
	connectCtx, stop := context.WithTimeout(ctx, 3*time.Second)
	conn, err := pgx.ConnectConfig(connectCtx, e.DB.Config().ConnConfig.Copy())
	stop()
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn.Close(closeCtx)
	}()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,71352))`, id).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return ErrBusy
	}
	account, err := e.Connections.Repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if account.Record.Kind != "ptsite" || !account.Record.Enabled {
		return ErrChanged
	}
	started := time.Now()
	trace.Info(ctx, "sites", "站点数据刷新开始", "accountId", id, "site", account.Record.Name)
	bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
	client, collectionErr := e.Connections.Site(bounded, id)
	var metric sites.Metrics
	if e.Connections.OutboundHTTP != nil {
		client.HTTP = e.Connections.OutboundHTTP
	}
	if collectionErr == nil {
		metric, collectionErr = client.Metrics(bounded)
	}
	cancel()
	revision, fingerprint := account.Record.Revision, ""
	if client.SourceRevision > 0 {
		revision, fingerprint = client.SourceRevision, client.DefinitionFingerprint
	}
	if err = e.Save(ctx, id, revision, fingerprint, started, metric, collectionErr); err != nil {
		return err
	}
	if collectionErr != nil {
		trace.Warn(ctx, "sites", "站点数据刷新失败，保留最近成功数据", "accountId", id, "error", collectionErr, "durationMs", time.Since(started).Milliseconds())
		return fmt.Errorf("%w: %v", ErrCollection, collectionErr)
	}
	trace.Info(ctx, "sites", "站点数据刷新完成", "accountId", id, "state", sites.MetricsState(metric), "durationMs", time.Since(started).Milliseconds())
	return nil
}

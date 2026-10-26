package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"media-tool/internal/platform/outbox"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/adapters/telegram"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
)

type Dispatcher struct {
	DB          *pgxpool.Pool
	Connections *integrations.Manager
}

var titles = map[string]string{"idle_update.result": "闲时自动更新结果", "idle_update.disabled": "闲时自动更新已关闭", "ops.alert.active": "系统运维告警", "ops.alert.recovered": "系统运维告警恢复", "ops.backup.failed": "系统备份失败", "ops.backup.ready": "系统备份完成", "ops.drill.failed": "恢复演练失败", "ops.drill.verified": "恢复演练通过", "ops.maintenance.resumed": "维护窗口解除", "files.batch.succeeded": "媒体整理批次已完成", "files.batch.partial": "媒体整理批次部分完成（需检查剩余项）", "files.batch.needs_reconcile": "媒体整理结果未知（需读回核对）", "files.succeeded": "文件整理已完成", "files.needs_reconcile": "文件整理需核对", "files.failed": "文件整理未完成", "library.emby.visible": "托管输出逐路径可见（待完整播放验收）", "plugin.notice": "插件通知", "download.started": "下载已开始", "download.completed": "本地下载完成", "media.imported": "媒体 STRM 已生成", "media.batch_ready": "媒体文件已准备入库", "library.refresh_requested": "Emby 媒体库刷新已请求", "library.visible": "媒体已在Emby库中可见（待客户端播放验收）", "transfer.failed": "媒体上传失败"}

func (d Dispatcher) Prepare(ctx context.Context) error {
	if _, err := d.DB.Exec(ctx, `UPDATE notification_deliveries SET state='outcome_unknown',last_error='Worker stopped before a confirmed send receipt; manual verification required'WHERE state='sending'AND started_at<now()-interval '2 minutes'`); err != nil {
		return err
	}
	hasModule := (pluginv2.Store{DB: d.DB}).HasNotificationModule(ctx)
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	rows, err := tx.Query(ctx, `SELECT id::text,event_type,payload FROM outbox_events WHERE dispatched_at IS NULL ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 100`)
	if err != nil {
		return err
	}
	type event struct {
		id, kind string
		payload  []byte
	}
	events := []event{}
	for rows.Next() {
		var e event
		if err = rows.Scan(&e.id, &e.kind, &e.payload); err != nil {
			rows.Close()
			return err
		}
		events = append(events, e)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	var telegramID string
	var telegramRevision int64
	if checkedErr := tx.QueryRow(ctx, `SELECT id::text,revision FROM integrations WHERE kind='telegram'AND enabled LIMIT 1`).Scan(&telegramID, &telegramRevision); checkedErr != nil && !errors.Is(checkedErr, pgx.ErrNoRows) {
		return checkedErr
	}
	for _, e := range events {
		title, notify := titles[e.kind]
		if notify {
			var data map[string]any
			if checkedErr := json.Unmarshal(e.payload, &data); checkedErr != nil {
				return checkedErr
			}
			name, _ := data["title"].(string)
			body := title
			if name != "" {
				body += "：" + name
			}
			for _, key := range []string{"batchId", "operationId", "assetId", "message"} {
				if value, ok := data[key].(string); ok && value != "" {
					body += "\n" + key + "：" + value
				}
			}
			if e.kind == "plugin.notice" {
				title = name
				if message, ok := data["message"].(string); ok {
					body = message
				}
			}
			var notificationID string
			if err = tx.QueryRow(ctx, `INSERT INTO notification_messages(id,event_id,title,body)VALUES($1,$2,$3,$4)ON CONFLICT(event_id)DO UPDATE SET title=notification_messages.title RETURNING id::text`, jobs.UUID(), e.id, title, body).Scan(&notificationID); err != nil {
				return err
			}
			origin := "builtin"
			if e.kind == "plugin.notice" {
				origin = "plugin"
			}
			if err = outbox.EmitVersionTx(ctx, tx, "notification.queued", notificationID, "queued-v1", contracts.NotificationQueued{NotificationID: notificationID, Origin: origin}); err != nil {
				return err
			}
			if telegramID == "" && hasModule {
				if _, err = tx.Exec(ctx, `INSERT INTO notification_deliveries(id,event_id)VALUES($1,$2)`, jobs.UUID(), e.id); err != nil {
					return err
				}
			}
			if telegramID != "" {
				if _, err = tx.Exec(ctx, `INSERT INTO notification_deliveries(id,event_id,integration_id,integration_revision)VALUES($1,$2,$3,$4)ON CONFLICT(event_id,integration_id)WHERE integration_id IS NOT NULL DO NOTHING`, jobs.UUID(), e.id, telegramID, telegramRevision); err != nil {
					return err
				}
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE outbox_events SET dispatched_at=now()WHERE id=$1`, e.id); err != nil {
			return err
		}
	}
	return dbutil.Commit(ctx, tx)
}
func (d Dispatcher) Deliver(ctx context.Context) error {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var id, connectionID, body, notificationID string
	var attempts int
	var savedRevision *int64
	err = tx.QueryRow(ctx, `SELECT n.id::text,coalesce(n.integration_id::text,''),m.body,n.attempts,n.integration_revision,m.id::text FROM notification_deliveries n JOIN notification_messages m ON m.event_id=n.event_id WHERE n.state IN ('pending','retry')AND next_attempt_at<=now() ORDER BY next_attempt_at FOR UPDATE OF n SKIP LOCKED LIMIT 1`).Scan(&id, &connectionID, &body, &attempts, &savedRevision, &notificationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	attemptToken := jobs.UUID()
	tag, err := tx.Exec(ctx, `UPDATE notification_deliveries SET state='sending',attempts=attempts+1,started_at=now(),attempt_token=$2 WHERE id=$1 AND state IN ('pending','retry')`, id, attemptToken)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("notification delivery attempt is no longer pending")
	}
	if err = dbutil.Commit(ctx, tx); err != nil {
		return err
	}
	// Revalidate the configured target before any provider sees the notification.
	if connectionID != "" {
		current, e := d.Connections.Repository.Default(ctx, "telegram")
		if e != nil || current.Record.ID != connectionID || savedRevision != nil && current.Record.Revision != *savedRevision {
			return d.finishAttempt(ctx, id, attemptToken, "failed", "", "Telegram target changed; manual retry required", 0)
		}
	}
	if d.Connections != nil {
		receipt, e := (pluginv2.Store{DB: d.DB, Vault: d.Connections.Repository.Vault}).ExternalEffect(ctx, "notifications.channel", contracts.EffectRequest{RequestID: attemptToken, ObjectID: notificationID, Kind: "notification"})
		if e != nil || receipt.State == "unknown" {
			return d.finishAttempt(ctx, id, attemptToken, "outcome_unknown", "", "Plugin outcome unknown; operator verification required", 0)
		}
		if receipt.State == "sent" {
			return d.finishAttempt(ctx, id, attemptToken, "sent", "plugin-confirmed", "", 0)
		}
	}
	if connectionID == "" {
		return d.finishAttempt(ctx, id, attemptToken, "failed", "", "No configured outbound provider", 0)
	}
	client, err := d.Connections.Telegram(ctx)
	if err != nil {
		sendErr := err
		if updateErr := d.finishAttempt(ctx, id, attemptToken, "failed", "", "Telegram connection unavailable", 0); updateErr != nil {
			return errors.Join(sendErr, updateErr)
		}
		return sendErr
	}
	current, err := d.Connections.Repository.Default(ctx, "telegram")
	if err != nil || current.Record.ID != connectionID || savedRevision != nil && current.Record.Revision != *savedRevision {
		targetErr := errors.New("Telegram target changed; manual retry required")
		if err != nil {
			targetErr = err
		}
		if updateErr := d.finishAttempt(ctx, id, attemptToken, "failed", "", "Telegram target changed; manual retry required", 0); updateErr != nil {
			return errors.Join(targetErr, updateErr)
		}
		return targetErr
	}
	message, retryAfter, sendErr := client.SendText(ctx, body)
	state, lastError := "sent", ""
	wait := time.Duration(1<<min(attempts+1, 10)) * time.Second
	if sendErr != nil {
		if retryAfter > 0 {
			state = "retry"
			wait = time.Duration(retryAfter) * time.Second
			lastError = "Telegram rate limit"
		} else if requestErr, ok := sendErr.(*telegram.RequestError); ok && !requestErr.Unknown {
			state = "failed"
			lastError = "Telegram explicitly refused delivery"
			if requestErr.Retryable {
				state = "retry"
				lastError = "Telegram temporarily unavailable"
			}
		} else {
			state = "outcome_unknown"
			lastError = "Delivery result unknown; manual retry may duplicate a message"
		}
	}
	if attempts >= 4 && state == "retry" {
		state = "failed"
	}
	return d.finishAttempt(ctx, id, attemptToken, state, message, lastError, wait.Milliseconds())
}

func (d Dispatcher) finishAttempt(ctx context.Context, id, attemptToken, state, providerMessageID, lastError string, delayMS int64) error {
	tag, err := d.DB.Exec(ctx, `UPDATE notification_deliveries SET state=$3,provider_message_id=NULLIF($4,''),last_error=$5,next_attempt_at=now()+($6::bigint*interval '1 millisecond') WHERE id=$1 AND state='sending' AND attempt_token=$2`, id, attemptToken, state, providerMessageID, lastError, delayMS)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("notification delivery attempt is no longer active")
	}
	return nil
}

type Message struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Body          string     `json:"body"`
	CreatedAt     time.Time  `json:"createdAt"`
	ReadAt        *time.Time `json:"readAt,omitempty"`
	DeliveryID    *string    `json:"deliveryId,omitempty"`
	DeliveryError string     `json:"deliveryError"`
	TelegramState string     `json:"telegramState"`
}

func (d Dispatcher) List(ctx context.Context) ([]Message, error) { return d.Page(ctx, 0, 200) }
func (d Dispatcher) Page(ctx context.Context, start, limit int) ([]Message, error) {
	rows, err := d.DB.Query(ctx, `SELECT m.id::text,m.title,m.body,m.created_at,m.read_at,coalesce(n.state,'not_configured'),n.id::text,coalesce(n.last_error,'') FROM notification_messages m LEFT JOIN notification_deliveries n ON n.event_id=m.event_id ORDER BY m.created_at DESC,m.id,n.id LIMIT $1 OFFSET $2`, limit, start)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if err = rows.Scan(&m.ID, &m.Title, &m.Body, &m.CreatedAt, &m.ReadAt, &m.TelegramState, &m.DeliveryID, &m.DeliveryError); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d Dispatcher) Retry(ctx context.Context, id string, acceptDuplicateRisk bool) error {
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	var state, connectionID string
	var revision *int64
	err = tx.QueryRow(ctx, `SELECT state,integration_id::text,integration_revision FROM notification_deliveries WHERE id=$1 FOR UPDATE`, id).Scan(&state, &connectionID, &revision)
	if err != nil {
		return err
	}
	if err = pluginOwnership(ctx, tx, id); err != nil {
		return err
	}
	if state != "failed" && state != "outcome_unknown" {
		return errors.New("delivery is not in a manually retryable state")
	}
	if state == "outcome_unknown" && !acceptDuplicateRisk {
		return errors.New("confirm possible duplicate risk before resending unknown outcome")
	}
	current, err := d.Connections.Repository.Default(ctx, "telegram")
	if err != nil || current.Record.ID != connectionID || revision != nil && current.Record.Revision != *revision {
		return errors.New("Telegram target/credentials changed; do not reuse the old event automatically")
	}
	if _, err = tx.Exec(ctx, `UPDATE notification_deliveries SET state='pending',started_at=NULL,next_attempt_at=now(),attempts=0,attempt_token='',last_error=''WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_logs(actor,action,details)VALUES('administrator','notification.retry',jsonb_build_object('deliveryId',$1::text,'duplicateRiskAccepted',$2::boolean))`, id, acceptDuplicateRisk); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}

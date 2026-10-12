package pluginhost

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/plugins"
)

func New(db *pgxpool.Pool, manager *integrations.Manager) plugins.HostFunc {
	return func(ctx context.Context, op string, raw json.RawMessage) (any, error) {
		switch op {
		case "library.list", "downloads.list":
			query := `SELECT id::text,title,media_type,metadata FROM media_items ORDER BY id LIMIT 50`
			if op == "downloads.list" {
				query = `SELECT id::text,download_state,seeding_state,details FROM download_tasks ORDER BY created_at DESC LIMIT 50`
			}
			rows, e := db.Query(ctx, query)
			if e != nil {
				return nil, e
			}
			defer rows.Close()
			out := []map[string]any{}
			fields := rows.FieldDescriptions()
			for rows.Next() {
				values, e := rows.Values()
				if e != nil {
					return nil, e
				}
				item := map[string]any{}
				for i, v := range values {
					item[fields[i].Name] = v
				}
				out = append(out, item)
			}
			return out, rows.Err()
		case "tmdb.search":
			var in struct {
				Keyword   string `json:"keyword"`
				MediaType string `json:"mediaType"`
			}
			if json.Unmarshal(raw, &in) != nil || in.Keyword == "" || len(in.Keyword) > 200 {
				return nil, errors.New("TMDB参数无效")
			}
			if in.MediaType == "" {
				in.MediaType = "auto"
			}
			if in.MediaType != "auto" && in.MediaType != "movie" && in.MediaType != "series" {
				return nil, errors.New("媒体类型无效")
			}
			client, e := manager.TMDB(ctx)
			if e != nil {
				return nil, e
			}
			return client.Search(ctx, in.MediaType, in.Keyword)
		case "notifications.send":
			var in struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			}
			if json.Unmarshal(raw, &in) != nil || len(in.Title) < 1 || len(in.Title) > 200 || len(in.Body) > 4000 {
				return nil, errors.New("通知参数无效")
			}
			payload, _ := json.Marshal(map[string]string{"title": in.Title, "message": in.Body})
			id := jobs.UUID()
			_, e := db.Exec(ctx, `INSERT INTO outbox_events(id,event_type,payload)VALUES($1,'plugin.notice',$2)`, id, payload)
			return map[string]string{"eventId": id}, e
		}
		return nil, errors.New("未支持的宿主能力")
	}
}

package dataview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	deferutil "media-tool/internal/platform/cleanup"
	"regexp"
	"strconv"
	"strings"
)

var ErrInvalid = errors.New("invalid data browser request")
var sensitive = regexp.MustCompile(`(?i)(password|secret|token|cookie|authorization|credential|ciphertext|nonce|private|digest|api.?key|passkey|recovery|access.?key)`)
var signed = regexp.MustCompile(`(?i)(https?://[^\s?"<>]+)\?[^\s"<>]+`)
var jwt = regexp.MustCompile(`\b[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{16,}\b`)

func Redact(key string, value any) any { return redactDepth(key, value, 0) }
func redactDepth(key string, value any, depth int) any {
	if depth > 10 {
		return "[深层数据已截断]"
	}
	if sensitive.MatchString(key) {
		return "[敏感字段已隐藏]"
	}
	switch v := value.(type) {
	case []byte:
		return "[二进制内容已隐藏]"
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			if len(out) == 100 {
				break
			}
			out[k] = redactDepth(k, x, depth+1)
		}
		return out
	case []string:
		out := []any{}
		for i, x := range v {
			if i == 100 {
				break
			}
			out = append(out, redactDepth(key, x, depth+1))
		}
		return out
	case []any:
		out := make([]any, 0, min(len(v), 100))
		for i, x := range v {
			if i == 100 {
				break
			}
			out = append(out, redactDepth(key, x, depth+1))
		}
		return out
	case string:
		if len(v) > 16384 {
			v = v[:16384] + " [已截断]"
		}
		var parsed any
		if strings.HasPrefix(strings.TrimSpace(v), "{") || strings.HasPrefix(strings.TrimSpace(v), "[") {
			if json.Unmarshal([]byte(v), &parsed) == nil {
				return redactDepth(key, parsed, depth+1)
			}
		}
		return jwt.ReplaceAllString(signed.ReplaceAllString(v, "$1?[参数已隐藏]"), "[认证内容已隐藏]")
	default:
		return v
	}
}

type Engine struct {
	DB    *pgxpool.Pool
	Redis *redis.Client
}
type Table struct {
	Name          string `json:"name"`
	EstimatedRows int64  `json:"estimatedRows"`
}

func (e Engine) Tables(ctx context.Context) ([]Table, error) {
	if e.DB == nil {
		return nil, ErrInvalid
	}
	rows, err := e.DB.Query(ctx, `SELECT c.relname,greatest(c.reltuples::bigint,0) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='r' ORDER BY c.relname LIMIT 256`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Table{}
	for rows.Next() {
		var t Table
		if err = rows.Scan(&t.Name, &t.EstimatedRows); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (e Engine) Rows(ctx context.Context, name string, page, size int) (map[string]any, error) {
	if page < 1 || page > 2000 || size < 1 || size > 50 {
		return nil, ErrInvalid
	}
	tables, err := e.Tables(ctx)
	if err != nil {
		return nil, err
	}
	found := false
	for _, t := range tables {
		if t.Name == name {
			found = true
		}
	}
	if !found {
		return nil, ErrInvalid
	}
	tx, err := e.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `SELECT set_config('statement_timeout','2500ms',true)`); err != nil {
		return nil, err
	}
	columns := []map[string]any{}
	columnRows, err := tx.Query(ctx, `SELECT column_name,data_type,udt_name FROM information_schema.columns WHERE table_schema='public'AND table_name=$1 ORDER BY ordinal_position`, name)
	if err != nil {
		return nil, err
	}
	for columnRows.Next() {
		var column, dataType, udt string
		if err = columnRows.Scan(&column, &dataType, &udt); err != nil {
			columnRows.Close()
			return nil, err
		}
		columns = append(columns, map[string]any{"name": column, "dataType": dataType, "udt": udt})
	}
	err = columnRows.Err()
	columnRows.Close()
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT to_jsonb(t) FROM "+pgx.Identifier{"public", name}.Sanitize()+" t ORDER BY ctid LIMIT $1 OFFSET $2", size, (page-1)*size)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			rows.Close()
			return nil, ErrInvalid
		}
		for key, value := range row {
			if text, ok := value.(string); ok && len(text) > 65536 {
				row[key] = text[:65536] + " [容量保护：已截断，未脱敏]"
			}
		}
		out = append(out, row)
	}

	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var count int64
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{"public", name}.Sanitize()).Scan(&count); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"table": name, "columns": columns, "items": out, "total": count, "page": page, "pageSize": size, "readOnly": true}, nil
}
func (e Engine) RedisKeys(ctx context.Context, cursor, pattern string) (map[string]any, error) {
	if e.Redis == nil {
		return nil, ErrInvalid
	}
	number, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil || len(pattern) > 200 {
		return nil, ErrInvalid
	}
	if pattern == "" {
		pattern = "*"
	}
	keys, next, err := e.Redis.Scan(ctx, number, pattern, 50).Result()
	if err != nil {
		return nil, err
	}
	if len(keys) > 200 {
		keys = keys[:200]
	}
	items := []map[string]any{}
	for _, k := range keys {
		t, _ := e.Redis.Type(ctx, k).Result()
		ttl, _ := e.Redis.TTL(ctx, k).Result()
		memory, _ := e.Redis.MemoryUsage(ctx, k).Result()
		items = append(items, map[string]any{"key": k, "type": t, "ttlSeconds": int64(ttl.Seconds()), "bytes": memory})
	}
	count, _ := e.Redis.DBSize(ctx).Result()
	return map[string]any{"items": items, "cursor": fmt.Sprint(next), "total": count, "readOnly": true}, nil
}
func (e Engine) RedisValue(ctx context.Context, key string) (map[string]any, error) {
	if e.Redis == nil || key == "" || len(key) > 512 {
		return nil, ErrInvalid
	}
	kind, err := e.Redis.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	ttl, _ := e.Redis.TTL(ctx, key).Result()
	size, _ := e.Redis.MemoryUsage(ctx, key).Result()
	var value any
	truncated := false
	if size > 4<<20 && kind != "string" {
		value = "集合过大，仅显示元数据"
		truncated = true

	} else {
		switch kind {
		case "string":
			raw, err := e.Redis.GetRange(ctx, key, 0, 8192).Result()
			if err != nil {
				return nil, err
			}
			truncated = len(raw) > 8192
			if truncated {
				raw = raw[:8192]
			}
			value = RawValue(raw)
		case "list":
			raw, err := e.Redis.LRange(ctx, key, 0, 49).Result()
			if err != nil {
				return nil, err
			}
			out := []any{}
			for _, x := range raw {
				out = append(out, RawValue(x))
			}
			value = out
			n, _ := e.Redis.LLen(ctx, key).Result()
			truncated = n > 50
		case "hash":
			raw, next, err := e.Redis.HScan(ctx, key, 0, "*", 20).Result()
			if err != nil {
				return nil, err
			}
			out := map[string]any{}
			bytes := 0
			for i := 0; i+1 < len(raw) && i < 100; i += 2 {
				bytes += len(raw[i]) + len(raw[i+1])
				if bytes > 16384 {
					truncated = true
					break
				}
				out[raw[i]] = RawValue(raw[i+1])
			}
			value = out
			truncated = truncated || next != 0 || len(raw) > 100
		case "set":
			raw, next, err := e.Redis.SScan(ctx, key, 0, "*", 20).Result()
			if err != nil {
				return nil, err
			}
			out := []any{}
			for i, x := range raw {
				if i == 50 {
					break
				}
				out = append(out, RawValue(x))
			}
			value = out
			truncated = next != 0 || len(raw) > 50
		case "zset":
			raw, err := e.Redis.ZRangeWithScores(ctx, key, 0, 49).Result()
			if err != nil {
				return nil, err
			}
			out := []any{}
			for _, x := range raw {
				out = append(out, map[string]any{"score": x.Score, "value": RawValue(x.Member)})
			}
			value = out
			n, _ := e.Redis.ZCard(ctx, key).Result()
			truncated = n > 50
		case "none":
			value = "键已过期或不存在"
		default:
			value = "此类型仅显示元数据"
		}
	}
	return map[string]any{"key": key, "type": kind, "ttlSeconds": int64(ttl.Seconds()), "bytes": size, "value": value, "truncated": truncated, "readOnly": true}, nil
}

func RawValue(value any) any {
	if raw, ok := value.(string); ok {
		var parsed any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			return parsed
		}
	}
	return value
}

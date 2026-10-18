package sites

import (
	"context"
	"errors"
	deferutil "media-tool/internal/platform/cleanup"
	dbutil "media-tool/internal/platform/database"
	"runtime/debug"

	mediatoolsite "github.com/dingding229/mediatool-site"
	"github.com/jackc/pgx/v5"
)

func GoTemplates() []mediatoolsite.Template               { return mediatoolsite.All() }
func GoTemplate(id string) (mediatoolsite.Template, bool) { return mediatoolsite.Get(id) }
func TemplateBuildVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, d := range info.Deps {
			if d.Path == "github.com/dingding229/mediatool-site" {
				return d.Version
			}
		}
	}
	return "compiled-go"
}

// RegisterGoTemplates refreshes selected definitions and preserves templates required
// by existing accounts. Merely appearing in the compiled catalog never loads a site.
func (r Repository) RegisterGoTemplates(ctx context.Context) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if _, err = tx.Exec(ctx, `INSERT INTO site_template_loads(template_id)
 SELECT DISTINCT settings->>'definitionId' FROM integrations WHERE kind='ptsite'
 AND settings->>'definitionId'=ANY($1::text[]) ON CONFLICT DO NOTHING`, templateIDs()); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT template_id FROM site_template_loads ORDER BY template_id`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	if err = registerTemplates(ctx, tx, ids); err != nil {
		return err
	}
	return dbutil.Commit(ctx, tx)
}

func templateIDs() []string {
	ids := []string{}
	for _, t := range GoTemplates() {
		ids = append(ids, t.ID)
	}
	return ids
}

// LoadGoTemplates atomically loads only the requested templates; repeated loads are idempotent.
func (r Repository) LoadGoTemplates(ctx context.Context, ids []string) error {
	if len(ids) == 0 || len(ids) > len(GoTemplates()) {
		return errors.New("select valid templates")
	}
	for _, id := range ids {
		if _, ok := GoTemplate(id); !ok {
			return errors.New("unknown template")
		}
	}
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer deferutil.Rollback(ctx, tx)
	if err = registerTemplates(ctx, tx, ids); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.Exec(ctx, `INSERT INTO site_template_loads(template_id) VALUES($1) ON CONFLICT DO NOTHING`, id); err != nil {
			return err
		}
	}
	return dbutil.Commit(ctx, tx)
}
func registerTemplates(ctx context.Context, tx pgx.Tx, ids []string) error {
	for _, id := range ids {
		t, ok := GoTemplate(id)
		if !ok {
			continue
		}
		raw, err := t.Snapshot()
		if err != nil {
			return err
		}
		d, err := prepareDefinition(raw, []byte(`{}`), "go:"+TemplateBuildVersion())
		if err != nil {
			return err
		}
		query := definitionInsert + ` WHERE site_definitions.definition IS DISTINCT FROM excluded.definition OR site_definitions.source_commit IS DISTINCT FROM excluded.source_commit OR site_definitions.source_repository IS DISTINCT FROM excluded.source_repository`
		if _, err = tx.Exec(ctx, query, definitionArguments(d)...); err != nil {
			return err
		}
	}
	return nil
}
func (r Repository) LoadedGoTemplates(ctx context.Context) (map[string]bool, error) {
	rows, err := r.DB.Query(ctx, `SELECT template_id FROM site_template_loads`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	loaded := map[string]bool{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		loaded[id] = true
	}
	return loaded, rows.Err()
}
func GoTemplateCatalog(loaded map[string]bool) []map[string]any {
	out := []map[string]any{}
	for _, t := range GoTemplates() {
		out = append(out, map[string]any{"id": t.ID, "name": t.Name, "domain": t.Domain, "version": t.Version, "iconUrl": "/api/v1/pt/templates/" + t.ID + "/icon?v=" + t.Version, "fields": t.Fields, "loaded": loaded[t.ID]})
	}
	return out
}
func SnapshotForTest(id string) []byte { t, _ := GoTemplate(id); raw, _ := t.Snapshot(); return raw }

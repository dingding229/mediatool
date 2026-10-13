package sites

import (
	"context"
	mediatoolsite "github.com/dingding229/mediatool-site"
	"runtime/debug"
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
func (r Repository) RegisterGoTemplates(ctx context.Context) error {
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	for _, t := range GoTemplates() {
		raw, e := t.Snapshot()
		if e != nil {
			return e
		}
		definition, e := prepareDefinition(raw, []byte(`{}`), "go:"+TemplateBuildVersion())
		if e != nil {
			return e
		}
		// Avoid changing timestamps/fingerprints on every startup with the same build.
		query := definitionInsert + ` WHERE site_definitions.definition IS DISTINCT FROM excluded.definition OR site_definitions.source_commit IS DISTINCT FROM excluded.source_commit OR site_definitions.source_repository IS DISTINCT FROM excluded.source_repository`
		if _, e = tx.Exec(ctx, query, definitionArguments(definition)...); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func GoTemplateCatalog() []map[string]any {
	out := []map[string]any{}
	for _, t := range GoTemplates() {
		out = append(out, map[string]any{"id": t.ID, "name": t.Name, "domain": t.Domain, "fields": t.Fields})
	}
	return out
}
func SnapshotForTest(id string) []byte { t, _ := GoTemplate(id); raw, _ := t.Snapshot(); return raw }

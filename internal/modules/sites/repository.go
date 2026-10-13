package sites

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ DB *pgxpool.Pool }

var ErrImportCommitUnconfirmed = errors.New("site import commit unconfirmed; inspect saved snapshot before retry")

func (r Repository) Get(ctx context.Context, id string) (Definition, error) {
	var d Definition
	var compatibility []byte
	err := r.DB.QueryRow(ctx, `SELECT id,name,schema_name,source_commit,definition,common_definition,compatibility,source_repository FROM site_definitions WHERE id=$1`, id).Scan(&d.ID, &d.Name, &d.Schema, &d.SourceCommit, &d.RawJSON, &d.CommonJSON, &compatibility, &d.SourceRepository)
	if err != nil {
		return d, errors.New("site definition unavailable; import the matching configuration")
	}
	json.Unmarshal(compatibility, &d.Compatibility)
	d.refreshCompatibility()
	return d, nil
}
func (r Repository) List(ctx context.Context) ([]Definition, error) {
	rows, err := r.DB.Query(ctx, `SELECT id,name,schema_name,source_commit,definition,common_definition,compatibility,source_repository FROM site_definitions ORDER BY name`)
	if err != nil {
		return nil, errors.New("site definition storage unavailable")
	}
	defer rows.Close()
	out := []Definition{}
	for rows.Next() {
		var d Definition
		var raw []byte
		if err = rows.Scan(&d.ID, &d.Name, &d.Schema, &d.SourceCommit, &d.RawJSON, &d.CommonJSON, &raw, &d.SourceRepository); err != nil {
			return nil, err
		}
		json.Unmarshal(raw, &d.Compatibility)
		d.refreshCompatibility()
		out = append(out, d)
	}
	return out, rows.Err()
}
func (d *Definition) refreshCompatibility() {
	if d.SourceRepository != NativeRepository {
		d.Compatibility = Map{"state": "unsupported", "notes": []string{"旧来源模板已停用，请从内置mediatool-site仓库重新导入"}, "issueCodes": []string{"unsupported_template_source"}, "accountVerified": false, "source": d.SourceRepository}
		return
	}
	configuration, err := DecodeStored(d.RawJSON, d.CommonJSON)
	if err != nil {
		note := "stored configuration invalid"
		code := "invalid_stored_definition"
		if errors.Is(err, ErrCaseOrderUnavailable) {
			note = "stored selector case priority lost; re-import the fixed snapshot"
			code = "selector_priority_unavailable"
		}
		d.Compatibility = Map{"state": "partial", "notes": []string{note}, "issueCodes": []string{code}, "accountVerified": false, "source": NativeRepository}
		return
	}
	d.Compatibility = Compatibility(configuration)
}

const definitionInsert = `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,compatibility,source_repository)VALUES($1,$2,$3,$4,$5,$6,$7,$8)ON CONFLICT(id)DO UPDATE SET name=excluded.name,schema_name=excluded.schema_name,source_commit=excluded.source_commit,definition=excluded.definition,common_definition=excluded.common_definition,compatibility=excluded.compatibility,source_repository=excluded.source_repository,updated_at=now()`

func prepareDefinition(raw, common []byte, commit string) (Definition, error) {
	raw, err := canonicalDefinitionJSON(raw)
	if err != nil {
		return Definition{}, err
	}
	common, err = canonicalDefinitionJSON(common)
	if err != nil {
		return Definition{}, err
	}
	config, err := DecodeStored(raw, common)
	if err != nil {
		return Definition{}, err
	}
	return Definition{ID: config.ID, Name: config.Name, Schema: config.Schema, SourceCommit: commit, SourceRepository: NativeRepository, RawJSON: raw, CommonJSON: common, Compatibility: Compatibility(config)}, nil
}
func definitionArguments(definition Definition) []any {
	compatibility, _ := json.Marshal(definition.Compatibility)
	return []any{definition.ID, definition.Name, definition.Schema, definition.SourceCommit, json.RawMessage(definition.RawJSON), json.RawMessage(definition.CommonJSON), compatibility, definition.SourceRepository}
}
func (r Repository) Save(ctx context.Context, raw, common []byte, commit string) (Definition, error) {
	definition, err := prepareDefinition(raw, common, commit)
	if err != nil {
		return Definition{}, err
	}
	if _, err = r.DB.Exec(ctx, definitionInsert, definitionArguments(definition)...); err != nil {
		return Definition{}, errors.New("site definition save failed")
	}
	return r.Get(ctx, definition.ID)
}

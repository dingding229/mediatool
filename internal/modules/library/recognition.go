package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/metadata"
	pluginv2 "media-tool/internal/modules/plugins/v2"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/modules/storage"
	"path"
	"strings"
	"unicode"
)

type Recognition struct {
	Candidate         Candidate        `json:"candidate"`
	Hint              releasename.Hint `json:"hint"`
	Matches           []metadata.Item  `json:"matches"`
	Selected          *metadata.Item   `json:"selected,omitempty"`
	Evidence          []string         `json:"evidence"`
	FileDigest        string           `json:"fileDigest"`
	NeedsConfirmation bool             `json:"needsConfirmation"`
}

func evidenceDigest(file storage.File) string {
	raw, _ := json.Marshal(file)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func titleKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func (e Engine) observe(ctx context.Context, connection, p string) (storage.File, error) {
	if e.Observe != nil {
		return e.Observe(ctx, connection, p)
	}
	client, err := e.Connections.CloudByID(ctx, connection)
	if err != nil {
		return storage.File{}, err
	}
	return client.Find(ctx, p)
}
func (e Engine) candidateFile(ctx context.Context, id string) (Candidate, string, storage.File, error) {
	var c Candidate
	var connection string
	var f storage.File
	var raw []byte
	err := e.DB.QueryRow(ctx, `SELECT c.id::text,c.file_id::text,f.storage_id::text,f.full_path,f.size_bytes,c.state,c.revision,f.state,c.updated_at,coalesce(c.asset_id::text,''),coalesce(f.provider_file_id,''),f.metadata FROM library_candidates c JOIN cloud_files f ON f.id=c.file_id WHERE c.id=$1`, id).Scan(&c.ID, &c.FileID, &connection, &c.Path, &c.Size, &c.State, &c.Revision, &c.CloudState, &c.UpdatedAt, &c.AssetID, &f.ID, &raw)
	if err != nil {
		return c, connection, f, err
	}
	c.Name = path.Base(c.Path)
	f.Path = c.Path
	f.Name = c.Name
	f.Size = c.Size
	if checkedErr := json.Unmarshal(raw, &f.Hashes); checkedErr != nil {
		return Candidate{}, "", storage.File{}, checkedErr
	}
	return c, connection, f, nil
}
func fileMatches(a, b storage.File) bool {
	if a.ID == "" || a.ID != b.ID || a.Path != b.Path || a.Size != b.Size || b.IsDirectory {
		return false
	}
	for k, v := range a.Hashes {
		if v != "" && !strings.EqualFold(v, b.Hashes[k]) {
			return false
		}
	}
	return true
}
func (e Engine) Recognize(ctx context.Context, id string) (Recognition, error) {
	c, connection, indexed, err := e.candidateFile(ctx, id)
	out := Recognition{Candidate: c, Hint: releasename.Parse(c.Name), Matches: []metadata.Item{}, Evidence: []string{}, NeedsConfirmation: true}
	if err != nil {
		return out, err
	}
	if e.Connections != nil {
		out.Hint = (pluginv2.Store{DB: e.DB, Vault: e.Connections.Repository.Vault}).RecognizeName(ctx, c.Name)
	}
	actual, err := e.observe(ctx, connection, c.Path)
	if err != nil {
		return out, err
	}
	if c.CloudState != "present" || !fileMatches(indexed, actual) {
		return out, errors.New("candidate file changed; complete a new scan before recognition")
	}
	out.FileDigest = evidenceDigest(indexed)
	out.Evidence = append(out.Evidence, "当前文件ID、路径和大小与索引相符")
	provider, err := e.Connections.TMDB(ctx)
	if err != nil {
		return out, err
	}
	kind := out.Hint.MediaType
	if kind == "unknown" {
		kind = "auto"
	}
	out.Matches, err = provider.Search(ctx, kind, out.Hint.Title)
	if err != nil {
		return out, err
	}
	var exact []metadata.Item
	for _, item := range out.Matches {
		if item.Year == out.Hint.Year && out.Hint.Year != "" && item.MediaType == out.Hint.MediaType && (titleKey(item.Title) == titleKey(out.Hint.Title) || titleKey(item.OriginalTitle) == titleKey(out.Hint.Title)) {
			exact = append(exact, item)
		}
	}
	if len(exact) == 1 {
		out.Selected = &exact[0]
		out.Evidence = append(out.Evidence, "唯一标题（或原名）+年份+媒体类型匹配；仍需确认身份和实际季集")
	} else {
		out.Evidence = append(out.Evidence, "无唯一精确匹配，请人工选择身份")
	}
	if out.Hint.MediaType == "series" && len(out.Hint.Episodes) == 0 {
		out.Evidence = append(out.Evidence, "季集覆盖未知，禁止自动入库")
	}
	return out, nil
}

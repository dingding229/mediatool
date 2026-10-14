package library

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/metadata"
)

type OutputProjection struct {
	Path              string `json:"path"`
	Episodes          []int  `json:"episodes,omitempty"`
	SourceEpisodes    []int  `json:"sourceEpisodes,omitempty"`
	WholeFilePlayback bool   `json:"wholeFilePlayback"`
}

// Emby 4.10 treats E01E03 as an inclusive range, even with an episode NFO.
// Sparse files therefore project into explicit single-episode entries, all
// referencing the same unchanged file/grant, not a guessed playback segment.
func AssetOutputProjections(cloudPath, assetID string, item metadata.Item, season *int, episodes []int) ([]OutputProjection, error) {
	if len(assetID) < 8 || item.Title == "" || item.MediaType != "movie" && item.MediaType != "series" {
		return nil, errors.New("asset identity and title required for output projection")
	}
	folder := safeName(item.Title)
	if item.Year != "" {
		folder += " (" + item.Year + ")"
	}
	edition := assetID[:8]
	if item.MediaType != "series" {
		filename := strings.TrimSuffix(safeName(path.Base(cloudPath)), path.Ext(cloudPath))
		return []OutputProjection{{Path: path.Join("Movies", folder, filename+" ["+edition+"].strm"), WholeFilePlayback: true}}, nil
	}
	if season == nil {
		return nil, errors.New("explicit season required")
	}
	sorted, err := metadata.NormalizeEpisodes(*season, episodes)
	if err != nil {
		return nil, err
	}
	continuous := true
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[i-1]+1 {
			continuous = false
		}
	}
	groups := [][]int{sorted}
	if !continuous {
		groups = nil
		for _, n := range sorted {
			groups = append(groups, []int{n})
		}
	}
	out := []OutputProjection{}
	for _, coverage := range groups {
		tag, err := metadata.EpisodeTag(*season, coverage)
		if err != nil {
			return nil, err
		}
		out = append(out, OutputProjection{Path: path.Join("TV", folder, fmt.Sprintf("Season %02d", *season), fmt.Sprintf("%s - %s [%s].strm", safeName(item.Title), tag, edition)), Episodes: append([]int(nil), coverage...), SourceEpisodes: append([]int(nil), sorted...), WholeFilePlayback: true})
	}
	return out, nil
}

func nfoProjectionFiles(output, assetID string, item metadata.Item, season *int, coverage, sourceEpisodes []int) ([]nfoContent, error) {
	files, err := nfoFilesCoverage(output, assetID, item, season, coverage)
	if err != nil {
		return nil, err
	}
	if len(sourceEpisodes) <= 1 || reflect.DeepEqual(coverage, sourceEpisodes) {
		return files, nil
	}
	text := []string{}
	for _, n := range sourceEpisodes {
		text = append(text, fmt.Sprint(n))
	}
	for i := range files {
		if files[i].scope != "asset:"+assetID {
			continue
		}
		var info nfo
		if err := xml.Unmarshal(files[i].data, &info); err != nil {
			return nil, err
		}
		info.Plot = "同一合集文件包含第" + strings.Join(text, ",") + "集。本条目提供所标集号的媒体库索引；播放读取整个原文件，不切片、不按单集跳转。\n\n" + info.Plot
		// Episode broadcast runtimes are not the duration of this full-file alias.
		info.Runtime = nil
		raw, err := xml.MarshalIndent(info, "", "  ")
		if err != nil {
			return nil, err
		}
		files[i].data = append([]byte(xml.Header), raw...)
	}
	return files, nil
}
func StageNFOProjection(ctx context.Context, j Journal, tx pgx.Tx, root, assetID string, item metadata.Item, season *int, projection OutputProjection) ([]string, error) {
	parts, err := nfoProjectionFiles(projection.Path, assetID, item, season, projection.Episodes, projection.SourceEpisodes)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, p := range selectedNFO(parts, assetID, item) {
		id, err := j.Stage(ctx, tx, root, p.relative, assetID, p.scope, "nfo", p.data)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}
func StageOutputProjections(ctx context.Context, j Journal, tx pgx.Tx, root, assetID string, item metadata.Item, season *int, projections []OutputProjection, content []byte, keyReference string) ([]string, error) {
	if len(projections) < 1 || len(projections) > 100 {
		return nil, errors.New("invalid projection count")
	}
	if err := CheckOutputLayout(ctx, tx, assetID, projections); err != nil {
		return nil, err
	}
	intents := []string{}
	totalBytes := int64(len(content)) * int64(len(projections))
	if totalBytes > 64<<20 {
		return nil, errors.New("output projection exceeds content limit")
	}
	for _, p := range projections {
		var occupied string
		err := tx.QueryRow(ctx, `SELECT asset_id::text FROM strm_entries WHERE output_path=$1 FOR UPDATE`, p.Path).Scan(&occupied)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if occupied != "" && occupied != assetID {
			return nil, ErrArtifactConflict
		}
		id, err := j.Stage(ctx, tx, root, p.Path, assetID, "asset:"+assetID, "strm", content)
		if err != nil {
			return nil, err
		}
		intents = append(intents, id)
		if _, err = tx.Exec(ctx, `INSERT INTO strm_entries(id,asset_id,output_path,key_reference)VALUES($1,$2,$3,$4) ON CONFLICT(output_path)DO UPDATE SET updated_at=now()WHERE strm_entries.asset_id=excluded.asset_id`, jobs.UUID(), assetID, p.Path, keyReference); err != nil {
			return nil, err
		}
		ids, err := StageNFOProjection(ctx, j, tx, root, assetID, item, season, p)
		if err != nil {
			return nil, err
		}
		intents = append(intents, ids...)
		extra, err := StageScrapeExtras(ctx, j, tx, root, assetID, item, season, p)
		if err != nil {
			return nil, err
		}
		intents = append(intents, extra...)
	}
	return uniqueIntents(intents), nil
}
func uniqueIntents(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func correctionProjections(outputs []oldOutput, season int, episodes []int) (map[string]OutputProjection, error) {
	wanted, err := metadata.NormalizeEpisodes(season, episodes)
	if err != nil {
		return nil, err
	}
	mapped := map[string]OutputProjection{}
	found := map[int]bool{}
	for _, o := range outputs {
		actualSeason, coverage, ok := metadata.Episodes(path.Base(o.Path))
		if !ok || actualSeason != season {
			return nil, errors.New("季集变化或旧命名需要明确预览并迁移输出，不能仅改NFO")
		}
		for _, n := range coverage {
			match := false
			for _, target := range wanted {
				if n == target {
					match = true
					break
				}
			}
			if !match {
				return nil, errors.New("旧输出包含未声明集号，请明确预览并迁移输出")
			}
			found[n] = true
		}
		// Emby expands all multi-number tags as a range, regardless of our parser.
		for i := 1; i < len(coverage); i++ {
			if coverage[i] != coverage[i-1]+1 {
				return nil, errors.New("非连续合集旧命名会被Emby补成范围，请明确预览并迁移输出")
			}
		}
		mapped[o.Path] = OutputProjection{Path: o.Path, Episodes: coverage, SourceEpisodes: wanted, WholeFilePlayback: true}
	}
	if len(found) != len(wanted) {
		return nil, errors.New("现有输出未完整覆盖确认集号，请明确预览并迁移输出")
	}
	return mapped, nil
}

// Existing approved output names may deliberately retain an edition/folder;
// coverage, not a newly computed basename, determines import-only resumption.
func ExistingOutputProjections(paths []string, mediaType string, season *int, episodes []int) ([]OutputProjection, error) {
	if len(paths) < 1 || len(paths) > 100 {
		return nil, ErrOutputLayoutRequired
	}
	if mediaType == "movie" {
		out := []OutputProjection{}
		for _, p := range paths {
			if !validRelativeArtifact(p) || !strings.HasPrefix(p, "Movies/") {
				return nil, ErrOutputLayoutRequired
			}
			out = append(out, OutputProjection{Path: p, WholeFilePlayback: true})
		}
		return out, nil
	}
	if mediaType != "series" || season == nil {
		return nil, ErrOutputLayoutRequired
	}
	old := []oldOutput{}
	for _, p := range paths {
		if !validRelativeArtifact(p) || !strings.HasPrefix(p, "TV/") {
			return nil, ErrOutputLayoutRequired
		}
		old = append(old, oldOutput{Path: p})
	}
	mapped, err := correctionProjections(old, *season, episodes)
	if err != nil {
		return nil, ErrOutputLayoutRequired
	}
	out := []OutputProjection{}
	seen := map[int]bool{}
	for _, p := range paths {
		plan := mapped[p]
		for _, n := range plan.Episodes {
			if seen[n] {
				return nil, ErrOutputLayoutRequired
			}
			seen[n] = true
		}
		out = append(out, plan)
	}
	return out, nil
}

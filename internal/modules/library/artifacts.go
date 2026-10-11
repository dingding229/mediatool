package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/platform/secrets"
)

type uniqueID struct {
	Type    string `xml:"type,attr"`
	Default string `xml:"default,attr"`
	Value   string `xml:",chardata"`
}
type nfo struct {
	XMLName       xml.Name
	Title         string    `xml:"title"`
	OriginalTitle string    `xml:"originaltitle,omitempty"`
	Year          string    `xml:"year,omitempty"`
	Plot          string    `xml:"plot,omitempty"`
	Season        *int      `xml:"season,omitempty"`
	Episode       *int      `xml:"episode,omitempty"`
	ShowTitle     string    `xml:"showtitle,omitempty"`
	Unique        *uniqueID `xml:"uniqueid,omitempty"`
	Premiered     string    `xml:"premiered,omitempty"`
	Aired         string    `xml:"aired,omitempty"`
	Runtime       *int      `xml:"runtime,omitempty"`
}

type nfoContent struct {
	relative, scope string
	data            []byte
}

func nfoFiles(output, assetID string, item metadata.Item, season, episode *int) ([]nfoContent, error) {
	coverage := []int{}
	if episode != nil {
		coverage = append(coverage, *episode)
	}
	return nfoFilesCoverage(output, assetID, item, season, coverage)
}
func nfoFilesCoverage(output, assetID string, item metadata.Item, season *int, episodes []int) ([]nfoContent, error) {
	data := nfo{XMLName: xml.Name{Local: "movie"}, Title: item.Title, OriginalTitle: item.OriginalTitle, Year: item.Year, Plot: item.Overview, Unique: &uniqueID{Type: "tmdb", Default: "true", Value: strconv.Itoa(item.ID)}}
	out := []nfoContent{}
	if item.MediaType == "series" {
		show := data
		show.XMLName = xml.Name{Local: "tvshow"}
		raw, err := xml.MarshalIndent(show, "", "  ")
		if err != nil {
			return nil, err
		}
		out = append(out, nfoContent{path.Join(path.Dir(path.Dir(output)), "tvshow.nfo"), "tmdb:series:" + strconv.Itoa(item.ID), append([]byte(xml.Header), raw...)})
		data.XMLName = xml.Name{Local: "episodedetails"}
		data.Unique = nil
		data.Season = season
		if len(episodes) > 0 {
			data.Episode = &episodes[0]
		}
		data.ShowTitle = item.Title
		selected := []metadata.Episode{}
		for _, number := range episodes {
			for _, e := range item.EpisodeDetails {
				if e.SeriesID == item.ID && season != nil && e.Season == *season && e.Number == number {
					selected = append(selected, e)
					break
				}
			}
		}
		if len(item.EpisodeDetails) > 0 && len(selected) != len(episodes) {
			return nil, errors.New("episode metadata does not match confirmed coverage")
		}
		if len(selected) > 0 {
			titles := []string{}
			plots := []string{}
			runtime := 0
			runtimeKnown := true
			for _, e := range selected {
				titles = append(titles, e.Title)
				if e.Overview != "" {
					plots = append(plots, fmt.Sprintf("S%02dE%02d: %s", e.Season, e.Number, e.Overview))
				}
				if e.Runtime <= 0 {
					runtimeKnown = false
				}
				runtime += e.Runtime
			}
			data.Title = strings.Join(titles, " / ")
			data.OriginalTitle = ""
			data.Plot = strings.Join(plots, "\n\n")
			data.Premiered = selected[0].AirDate
			data.Aired = selected[0].AirDate
			data.Year = ""
			if len(selected[0].AirDate) >= 4 {
				data.Year = selected[0].AirDate[:4]
			}
			if runtimeKnown {
				data.Runtime = &runtime
			}
			data.Unique = &uniqueID{Type: "tmdb", Default: "true", Value: strconv.Itoa(selected[0].ID)}
		}
	}
	raw, err := xml.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, err
	}
	out = append(out, nfoContent{strings.TrimSuffix(output, path.Ext(output)) + ".nfo", "asset:" + assetID, append([]byte(xml.Header), raw...)})
	return out, nil
}
func StageNFO(ctx context.Context, j Journal, tx pgx.Tx, root, output, assetID string, item metadata.Item, season, episode *int) ([]string, error) {
	coverage := []int{}
	if episode != nil {
		coverage = append(coverage, *episode)
	}
	return StageNFOCoverage(ctx, j, tx, root, output, assetID, item, season, coverage)
}
func StageNFOCoverage(ctx context.Context, j Journal, tx pgx.Tx, root, output, assetID string, item metadata.Item, season *int, episodes []int) ([]string, error) {
	files, err := nfoFilesCoverage(output, assetID, item, season, episodes)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, file := range files {
		id, err := j.Stage(ctx, tx, root, file.relative, assetID, file.scope, "nfo", file.data)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}
func WriteNFO(ctx context.Context, db *pgxpool.Pool, vault *secrets.Vault, root, output, assetID string, item metadata.Item, season, episode *int) error {
	j := Journal{DB: db, Vault: vault}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ids, err := StageNFO(ctx, j, tx, root, output, assetID, item, season, episode)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	for _, id := range ids {
		if err = j.Apply(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func writeManaged(ctx context.Context, db *pgxpool.Pool, vault *secrets.Vault, root, relative, assetID, kind string, content []byte) error {
	return (Journal{DB: db, Vault: vault}).Write(ctx, root, relative, assetID, "asset:"+assetID, kind, content)
}

// CopySubtitles stages an entire sidecar manifest before any output is written.
// VobSub index/data pairs, text subtitles and PGS sidecars retain language and
// forced/default suffixes. No source file is moved or deleted.
func CopySubtitles(ctx context.Context, db *pgxpool.Pool, vault *secrets.Vault, sourceRoot, relativeVideo, outputRoot, output, assetID string) error {
	return CopySubtitlesOutputs(ctx, db, vault, sourceRoot, relativeVideo, outputRoot, []string{output}, assetID)
}

type subtitlePart struct {
	relative string
	raw      []byte
}

func collectSubtitleOutputs(sourceRoot, relativeVideo string, outputs []string) ([]subtitlePart, error) {
	if !validRelativeArtifact(relativeVideo) || len(outputs) < 1 || len(outputs) > 100 {
		return nil, ErrArtifactConflict
	}
	outputStems := []string{}
	seenOutputs := map[string]bool{}
	for _, output := range outputs {
		if !validRelativeArtifact(output) || seenOutputs[output] {
			return nil, ErrArtifactConflict
		}
		seenOutputs[output] = true
		outputStems = append(outputStems, strings.TrimSuffix(output, path.Ext(output)))
	}
	root, err := os.OpenRoot(sourceRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory := path.Dir(relativeVideo)
	folder, err := root.Open(directory)
	if err != nil {
		return nil, err
	}
	entries, err := folder.ReadDir(-1)
	folder.Close()
	if err != nil {
		return nil, err
	}
	if len(entries) > 50000 {
		return nil, errors.New("subtitle directory exceeds safety limit")
	}
	videoStem := strings.TrimSuffix(path.Base(relativeVideo), path.Ext(relativeVideo))
	parts := []subtitlePart{}
	names := map[string]bool{}
	pairs := map[string]map[string]bool{}
	total := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(path.Ext(name))
		switch ext {
		case ".srt", ".ass", ".ssa", ".vtt", ".idx", ".sub", ".sup", ".smi", ".ttml":
		default:
			continue
		}
		stem := strings.TrimSuffix(name, path.Ext(name))
		if stem != videoStem && !strings.HasPrefix(stem, videoStem+".") {
			continue
		}
		suffix := strings.TrimPrefix(stem, videoStem)
		raw, err := readRegular(root, path.Join(directory, name))
		if err != nil {
			return nil, err
		}
		for _, outputStem := range outputStems {
			target := outputStem + suffix + ext
			if names[target] {
				return nil, errors.New("ambiguous subtitle output basename")
			}
			names[target] = true
			total += len(raw)
			if len(parts) >= 500 || total > 64<<20 {
				return nil, errors.New("subtitle projection manifest exceeds 500 files/64MiB")
			}
			parts = append(parts, subtitlePart{target, raw})
		}
		if ext == ".idx" || ext == ".sub" {
			if pairs[stem] == nil {
				pairs[stem] = map[string]bool{}
			}
			pairs[stem][ext] = true
		}
	}
	for _, pair := range pairs {
		if pair[".idx"] && !pair[".sub"] {
			return nil, errors.New("VobSub index lacks matching subtitle data")
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].relative < parts[j].relative })
	return parts, nil
}

// A read-only content proof; no DB intent or filesystem write occurs.
func SubtitleOutputsProof(sourceRoot, relativeVideo string, outputs []string) (string, error) {
	parts, err := collectSubtitleOutputs(sourceRoot, relativeVideo, outputs)
	if err != nil {
		return "", err
	}
	return subtitlePartsProof(parts)
}
func subtitlePartsProof(parts []subtitlePart) (string, error) {
	type proof struct{ Path, Digest string }
	values := []proof{}
	for _, p := range parts {
		digest := sha256.Sum256(p.raw)
		values = append(values, proof{p.relative, hex.EncodeToString(digest[:])})
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
func CopySubtitlesOutputs(ctx context.Context, db *pgxpool.Pool, vault *secrets.Vault, sourceRoot, relativeVideo, outputRoot string, outputs []string, assetID string) error {
	return CopySubtitlesOutputsVerified(ctx, db, vault, sourceRoot, relativeVideo, outputRoot, outputs, assetID, "")
}
func CopySubtitlesOutputsVerified(ctx context.Context, db *pgxpool.Pool, vault *secrets.Vault, sourceRoot, relativeVideo, outputRoot string, outputs []string, assetID, expectedProof string) error {
	parts, err := collectSubtitleOutputs(sourceRoot, relativeVideo, outputs)
	if err != nil {
		return err
	}
	if expectedProof != "" {
		actual, err := subtitlePartsProof(parts)
		if err != nil || actual != expectedProof {
			return ErrArtifactConflict
		}
	}
	if len(parts) == 0 {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	j := Journal{DB: db, Vault: vault}
	ids := []string{}
	for _, part := range parts {
		id, err := j.Stage(ctx, tx, outputRoot, part.relative, assetID, "asset:"+assetID, "subtitle", part.raw)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	for _, id := range ids {
		if err = j.Apply(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

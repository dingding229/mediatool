package library

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"github.com/jackc/pgx/v5"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/settings"
	"media-tool/internal/platform/outbox"
	"path"
	"strconv"
	"strings"
)

func (e Engine) enrichScraping(ctx context.Context, item metadata.Item, season *int) (metadata.Item, error) {
	prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
	if err != nil {
		return item, err
	}
	provider, err := e.Connections.TMDB(ctx)
	if err != nil {
		return item, err
	}
	return provider.EnrichScraping(ctx, item, season, prefs.Value.Scraping, prefs.Revision)
}
func checkScrapeSettings(ctx context.Context, tx pgx.Tx, item metadata.Item) error {
	if item.Scrape == nil {
		return nil
	}
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT revision FROM application_settings WHERE singleton FOR SHARE`).Scan(&revision); err != nil {
		return err
	}
	if revision != item.Scrape.SettingsRevision {
		return errors.New("scraping settings changed; preview again")
	}
	return nil
}
func saveScrapeReport(ctx context.Context, tx pgx.Tx, assetID string, item metadata.Item) error {
	if item.Scrape == nil {
		return nil
	}
	policy, _ := json.Marshal(item.Scrape.Policy)
	notice, _ := json.Marshal(item.Scrape.Notices)
	var revision int64
	err := tx.QueryRow(ctx, `UPDATE media_assets SET scrape_policy=$2,scrape_report=$3,scrape_settings_revision=$4 WHERE id=$1 RETURNING revision`, assetID, policy, notice, item.Scrape.SettingsRevision).Scan(&revision)
	if err != nil {
		return err
	}
	// Stable fact version includes persisted report/policy and asset revision; a
	// later scrape can produce a new fact, retries of the same snapshot cannot.
	digest := sha256.Sum256(append(policy, notice...))
	version := fmt.Sprintf("%d:%d:%x", revision, item.Scrape.SettingsRevision, digest)
	return outbox.EmitVersionTx(ctx, tx, "metadata.scraped", assetID, version, contracts.MetadataScraped{ObjectID: assetID, Kind: "asset", MediaIdentity: contracts.MediaIdentity{TMDBID: item.ID, MediaType: item.MediaType}, Revision: version})
}
func scrapePolicy(item metadata.Item) metadata.ScrapePolicy {
	if item.Scrape != nil {
		return item.Scrape.Policy
	}
	return metadata.DefaultScrapePolicy()
}
func selectedNFO(files []nfoContent, assetID string, item metadata.Item) []nfoContent {
	p := scrapePolicy(item)
	out := []nfoContent{}
	for _, file := range files {
		enabled := p.Movie.NFO
		if item.MediaType == "series" {
			enabled = p.Episode.NFO
			if file.scope != "asset:"+assetID {
				enabled = p.Series.NFO
			}
		}
		if enabled {
			out = append(out, file)
		}
	}
	return out
}
func scrapeFiles(output, assetID string, item metadata.Item, season *int, episodes []int) ([]scrapeFile, error) {
	out := []scrapeFile{}
	if item.Scrape == nil {
		return out, nil
	}
	directory, stem := path.Dir(output), strings.TrimSuffix(output, path.Ext(output))
	show := path.Dir(directory)
	policy := item.Scrape.Policy
	if item.MediaType == "series" && season != nil && policy.Season.NFO {
		title, plot := "Season "+strconv.Itoa(*season), ""
		if item.Scrape.Season != nil {
			title = item.Scrape.Season.Title
			plot = item.Scrape.Season.Overview
		}
		seasonID := 0
		if item.Scrape.Season != nil {
			seasonID = item.Scrape.Season.ID
		}
		value := nfo{XMLName: xml.Name{Local: "season"}, Title: title, Plot: plot, Season: season, Unique: &uniqueID{Type: "tmdb", Value: strconv.Itoa(seasonID)}}
		raw, err := xml.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}
		out = append(out, scrapeFile{Path: path.Join(directory, "season.nfo"), Scope: "tmdb:series:" + strconv.Itoa(item.ID) + ":season:" + strconv.Itoa(*season), Kind: "nfo", Data: append([]byte(xml.Header), raw...)})
	}
	for _, art := range item.Scrape.Artworks {
		name, scope := "", "asset:"+assetID
		role := art.Role
		if role == "backdrop" {
			role = "fanart"
		}
		switch art.Level {
		case "movie":
			name = stem + "-" + role + art.Extension
		case "series":
			name = path.Join(show, role+art.Extension)
			scope = "tmdb:series:" + strconv.Itoa(item.ID)
		case "season":
			if season == nil {
				return nil, errors.New("season required for artwork")
			}
			name = path.Join(directory, "poster"+art.Extension)
			scope = "tmdb:series:" + strconv.Itoa(item.ID) + ":season:" + strconv.Itoa(*season)
		case "episode":
			if len(episodes) == 0 || episodes[0] != art.Episode {
				continue
			}
			name = stem + "-thumb" + art.Extension
		default:
			return nil, errors.New("unsupported artwork scope")
		}
		out = append(out, scrapeFile{Path: name, Scope: scope, Kind: "artwork", Data: art.Data})
	}
	return out, nil
}

type scrapeFile struct {
	Path, Scope, Kind string
	Data              []byte
}

func StageScrapeExtras(ctx context.Context, j Journal, tx pgx.Tx, root, assetID string, item metadata.Item, season *int, p OutputProjection) ([]string, error) {
	files, err := scrapeFiles(p.Path, assetID, item, season, p.Episodes)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, file := range files {
		id, err := j.Stage(ctx, tx, root, file.Path, assetID, file.Scope, file.Kind, file.Data)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (e Engine) EnrichScraping(ctx context.Context, item metadata.Item, season *int) (metadata.Item, error) {
	return e.enrichScraping(ctx, item, season)
}
func PersistScrapeReport(ctx context.Context, tx pgx.Tx, asset string, item metadata.Item) error {
	if err := checkScrapeSettings(ctx, tx, item); err != nil {
		return err
	}
	return saveScrapeReport(ctx, tx, asset, item)
}

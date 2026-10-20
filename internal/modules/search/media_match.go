package search

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releasename"
	"media-tool/internal/platform/textkey"
	"strconv"
	"strings"
)

func normalizedTitle(s string) string { return textkey.Title(s) }

// The torrent's own release title/year/type are identity evidence. A subtitle
// mentioning a shared alias is never enough to bind an unrelated movie.
func MatchesMedia(r Resource, i metadata.Item) bool {
	return matchesMediaCandidate(r, i) && MediaYearWarning(r, i) == ""
}

func matchesMediaCandidate(r Resource, i metadata.Item) bool {
	if i.ID < 1 {
		return true
	}
	h := releasename.Parse(r.Title)
	if i.MediaType == "movie" {
		if h.MediaType == "series" || i.Year == "" || h.Year != i.Year {
			return false
		}
	} else if i.MediaType == "series" {
		if h.MediaType != "series" {
			return false
		}
	} else {
		return false
	}
	if raw, ok := r.Metadata["tmdbId"]; ok {
		value := strings.TrimSpace(strings.Trim(strings.TrimSpace(string(mustJSON(raw))), `"`))
		if value != strconv.Itoa(i.ID) {
			return false
		}
	}
	title := normalizedTitle(h.Title)
	a, b := normalizedTitle(i.Title), normalizedTitle(i.OriginalTitle)
	return title != "" && (title == a || b != "" && title == b || a != "" && b != "" && (title == a+b || title == b+a))
}

var ErrMediaYearConfirmation = errors.New("剧集资源年份与TMDB首播年不一致，可能为季年份或同名剧集；请人工核对并明确确认")

// A series release year may describe a season, not its premiere. Keep it visible
// in interactive results, but never silently upgrade it to confirmed identity.
func MediaYearWarning(r Resource, i metadata.Item) string {
	if i.MediaType != "series" || i.Year == "" {
		return ""
	}
	h := releasename.Parse(r.Title + ".mkv")
	if h.Year != "" && h.Year != i.Year {
		return "年份不一致，需人工确认：资源 " + h.Year + " / TMDB首播 " + i.Year
	}
	return ""
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (e Engine) MediaResource(ctx context.Context, runID, resourceID string, yearConfirmation ...bool) error {
	run, err := e.Get(ctx, runID, 1, 1)
	if err != nil {
		return err
	}
	var media metadata.Item
	if len(run.Media) > 0 && json.Unmarshal(run.Media, &media) != nil {
		return ErrSnapshotChanged
	}
	if media.ID == 0 {
		return nil
	}
	s, err := e.snapshot(ctx, runID, resourceID)
	if err != nil {
		return err
	}
	if MediaYearWarning(s.Resource, media) != "" && !(len(yearConfirmation) > 0 && yearConfirmation[0]) {
		return ErrMediaYearConfirmation
	}
	if !matchesMediaCandidate(s.Resource, media) {
		return errors.New("资源的发行标题、年份或类型与所选TMDB身份不匹配，请重新搜索；副标题同名不作为身份依据")
	}
	return nil
}

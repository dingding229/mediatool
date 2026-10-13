package subscriptions

import (
	"errors"
	"sort"

	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releaseattrs"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/subscriptionpolicy"
)

type Rules = subscriptionpolicy.Rules

type Input struct {
	MediaType       string `json:"mediaType"`
	TMDBID          int    `json:"tmdbId"`
	Enabled         bool   `json:"enabled"`
	Source          string `json:"source"`
	IntervalMinutes int    `json:"intervalMinutes"`
	Rules           Rules  `json:"rules"`
	Revision        int64  `json:"revision,omitempty"`
}

func Validate(in Input) error {
	for key, value := range map[string]string{"resolution": in.Rules.Resolution, "source": in.Rules.Source, "codec": in.Rules.Codec} {
		if !releaseattrs.Valid(key, value) {
			return errors.New("无效的结构化订阅筛选；请使用已支持的分辨率、来源和编码")
		}
	}

	if in.MediaType != "movie" && in.MediaType != "series" || in.TMDBID <= 0 {
		return errors.New("select a movie or series identity")
	}
	if in.Source != "search" && in.Source != "rss" && in.Source != "both" {
		return errors.New("source must be search, rss, or both")
	}
	if in.IntervalMinutes < 5 || in.IntervalMinutes > 1440 {
		return errors.New("interval must be 5..1440 minutes")
	}
	if len(in.Rules.AccountIDs) == 0 || len(in.Rules.AccountIDs) > 10 || len(in.Rules.Keyword) > 200 {
		return errors.New("select 1..10 accounts and a bounded search term")
	}
	seen := map[string]bool{}
	for _, id := range in.Rules.AccountIDs {
		if seen[id] {
			return errors.New("duplicate account")
		}
		seen[id] = true
	}
	if in.Rules.MinSize < 0 || in.Rules.MaxSize < 0 || in.Rules.MaxSize > 0 && in.Rules.MaxSize < in.Rules.MinSize || in.Rules.MinSeeders < 0 {
		return errors.New("invalid size or seeder rules")
	}
	if in.Rules.Season < 0 || in.Rules.Season > 999 || len(in.Rules.Episodes) > 2000 {
		return errors.New("invalid season/episode coverage")
	}
	for _, n := range in.Rules.Episodes {
		if n < 1 || n > 9999 {
			return errors.New("invalid episode number")
		}
	}
	if err := subscriptionpolicy.ValidateKeywords(in.Rules); err != nil {
		return err
	}
	return nil
}
func TitleMatches(title string, item metadata.Item) bool {
	return subscriptionpolicy.TitleMatches(title, item)
}
func Filter(resource search.Resource, item metadata.Item, rules Rules) (bool, []string) {
	return subscriptionpolicy.Filter(resource, item, rules)
}

func ManifestCoverage(names []string, season int) ([]int, error) {
	set := map[int]bool{}
	hasVideo := false
	for _, name := range names {
		if !metadata.IsVideo(name) || metadata.IsSample(name) {
			continue
		}
		hasVideo = true
		s, episodes, ok := metadata.Episodes(name)
		if !ok {
			return nil, errors.New("种子内视频季集不明确，需要人工确认")
		}
		if s != season {
			continue
		}
		for _, n := range episodes {
			set[n] = true
		}
	}
	if !hasVideo || len(set) == 0 {
		return nil, errors.New("种子内没有所需季的明确视频")
	}
	out := []int{}
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

package classification

import (
	"context"
	"encoding/json"
	"errors"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/releasename"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Rule struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Directory string   `json:"directory"`
	Enabled   bool     `json:"enabled"`
	GenreIDs  []int    `json:"genreIds"`
	Countries []string `json:"countries"`
	Languages []string `json:"languages"`
}
type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Directory string `json:"directory"`
	MediaType string `json:"mediaType"`
	GenreIDs  []int  `json:"genreIds"`
	Enabled   bool   `json:"enabled"`
	Rules     []Rule `json:"rules"`
}
type Policy struct {
	Groups []Group `json:"groups"`
}
type Record struct {
	Value    Policy `json:"value"`
	Revision int64  `json:"revision"`
}
type Match struct {
	GroupID   string `json:"groupId"`
	RuleID    string `json:"ruleId"`
	Name      string `json:"name"`
	Directory string `json:"directory"`
	Reason    string `json:"reason"`
}
type Repository struct{ DB *pgxpool.Pool }

var ErrConflict = errors.New("目录分类已被修改，请刷新后重试")
var identifier = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

func relative(s string) bool {
	return s != "" && len(s) <= 400 && !strings.HasPrefix(s, "/") && path.Clean(s) == s && s != "." && s != ".." && !strings.HasPrefix(s, "../") && !strings.Contains(s, "\\") && !strings.ContainsFunc(s, unicode.IsControl)
}
func (p Policy) Validate() error {
	if len(p.Groups) > 32 {
		return errors.New("分类组最多32个")
	}
	seen := map[string]bool{}
	for _, g := range p.Groups {
		if !identifier.MatchString(g.ID) || seen[g.ID] || g.Name == "" || len(g.Name) > 100 || !relative(g.Directory) || (g.MediaType != "movie" && g.MediaType != "series") || len(g.Rules) > 100 {
			return errors.New("分类组参数无效")
		}
		seen[g.ID] = true
		if len(g.GenreIDs) > 40 {
			return errors.New("分类组题材过多")
		}
		for _, id := range g.GenreIDs {
			if id < 1 || id > 100000 {
				return errors.New("分类组题材ID无效")
			}
		}
		for _, r := range g.Rules {
			if !identifier.MatchString(r.ID) || seen[r.ID] || r.Name == "" || len(r.Name) > 100 || !relative(r.Directory) || len(r.Countries) > 128 || len(r.Languages) > 40 || len(r.GenreIDs) > 40 {
				return errors.New("分类规则参数无效")
			}
			seen[r.ID] = true
			for _, c := range r.Countries {
				if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(c) {
					return errors.New("地区使用ISO两位代码")
				}
			}
			for _, c := range r.Languages {
				if !regexp.MustCompile(`^[a-z]{2,3}$`).MatchString(c) {
					return errors.New("语言使用原语言代码")
				}
			}
			for _, id := range r.GenreIDs {
				if id < 1 || id > 100000 {
					return errors.New("题材ID无效")
				}
			}
		}
	}
	return nil
}
func rule(id, name string, genres []int, countries, languages []string) Rule {
	return Rule{ID: id, Name: name, Directory: name, Enabled: true, GenreIDs: genres, Countries: countries, Languages: languages}
}
func Defaults() Policy {
	west := []string{"US", "GB", "BE", "CA", "CH", "CL", "CO", "CZ", "DE", "DK", "ES", "FI", "FR", "IT", "NL", "NO", "SE", "AT", "AU", "NZ", "IE"}
	asia := []string{"JP", "KR", "KP", "TH", "IN", "SG"}
	china := []string{"CN", "TW", "HK", "MO"}
	return Policy{Groups: []Group{
		{ID: "anime", Name: "动漫", Directory: "动漫", MediaType: "series", GenreIDs: []int{16}, Enabled: true, Rules: []Rule{rule("anime-jp", "日漫", nil, []string{"JP"}, nil), rule("anime-cn", "国漫", nil, china, nil), rule("anime-kr", "韩漫", nil, []string{"KR", "KP"}, nil), rule("anime-us", "美漫", nil, []string{"US"}, nil), rule("anime-other", "其他", nil, nil, nil)}},
		{ID: "movie", Name: "电影", Directory: "电影", MediaType: "movie", Enabled: true, Rules: []Rule{rule("movie-music", "演唱会", []int{10402}, nil, nil), rule("movie-documentary", "纪录片", []int{99}, nil, nil), rule("movie-animation", "动画电影", []int{16}, nil, nil), rule("movie-chinese", "华语电影", nil, nil, []string{"zh", "cn", "yue"}), rule("movie-asian", "日韩电影", nil, nil, []string{"ja", "ko"}), rule("movie-west", "欧美电影", nil, west, nil), rule("movie-other", "其他电影", nil, nil, nil)}},
		{ID: "series", Name: "电视剧", Directory: "电视剧", MediaType: "series", Enabled: true, Rules: []Rule{rule("series-doc", "纪录片", []int{99}, nil, nil), rule("series-kids", "儿童", []int{10762}, nil, nil), rule("series-variety", "综艺", []int{10764, 10767}, nil, nil), rule("series-cn", "国产剧", nil, china, nil), rule("series-asia", "日韩剧", nil, asia, nil), rule("series-west", "欧美剧", nil, west, nil), rule("series-other", "其他剧", nil, nil, nil)}},
	}}
}
func contains(a []string, s string) bool {
	for _, x := range a {
		if x == s {
			return true
		}
	}
	return false
}
func genres(want []int, item metadata.Item) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		for _, id := range item.GenreIDs {
			if w == id {
				return true
			}
		}
	}
	return false
}
func (p Policy) Match(i metadata.Item) Match {
	for _, g := range p.Groups {
		if !g.Enabled || g.MediaType != i.MediaType || !genres(g.GenreIDs, i) {
			continue
		}
		hasEnabledRules := false
		for _, r := range g.Rules {
			if r.Enabled {
				hasEnabledRules = true
			}
			if !r.Enabled || !genres(r.GenreIDs, i) {
				continue
			}
			if len(r.Countries) > 0 {
				ok := false
				for _, c := range i.Countries {
					if contains(r.Countries, c) {
						ok = true
					}
				}
				if !ok {
					continue
				}
			}
			if len(r.Languages) > 0 && !contains(r.Languages, i.OriginalLanguage) {
				continue
			}
			return Match{GroupID: g.ID, RuleID: r.ID, Name: g.Name + " / " + r.Name, Directory: path.Join(g.Directory, r.Directory), Reason: "按有序规则首次匹配；类型及不同字段AND，同字段多值OR"}
		}
		if !hasEnabledRules {
			return Match{GroupID: g.ID, Name: g.Name, Directory: g.Directory, Reason: "分组没有启用子规则，使用分组目录"}
		}
	}
	dir := "电影"
	if i.MediaType == "series" {
		dir = "电视剧"
	}
	return Match{Directory: dir, Name: dir, Reason: "无启用规则命中，使用类型兜底"}
}
func normalize(p *Policy) {
	if p.Groups == nil {
		p.Groups = []Group{}
	}
	for gi := range p.Groups {
		g := &p.Groups[gi]
		if g.GenreIDs == nil {
			g.GenreIDs = []int{}
		}
		if g.Rules == nil {
			g.Rules = []Rule{}
		}
		for ri := range g.Rules {
			r := &g.Rules[ri]
			if r.GenreIDs == nil {
				r.GenreIDs = []int{}
			}
			if r.Countries == nil {
				r.Countries = []string{}
			}
			if r.Languages == nil {
				r.Languages = []string{}
			}
		}
	}
}
func DecodeRecord(raw []byte, revision int64) (Record, error) {
	out := Record{Value: Defaults(), Revision: revision}
	if string(raw) != "null" {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return out, err
		}
		// Online compatibility: missing/null groups means the default policy remains.
		if groups, ok := object["groups"]; ok && string(groups) != "null" {
			if err := json.Unmarshal(raw, &out.Value); err != nil {
				return out, err
			}
		} else {
			delete(object, "groups")
			sanitized, err := json.Marshal(object)
			if err != nil {
				return out, err
			}
			if err = json.Unmarshal(sanitized, &out.Value); err != nil {
				return out, err
			}
		}
	}
	normalize(&out.Value)
	return out, out.Value.Validate()
}
func (r Repository) Get(ctx context.Context) (Record, error) {
	if r.DB == nil {
		return DecodeRecord([]byte("null"), 1)
	}
	var raw []byte
	var revision int64
	if err := r.DB.QueryRow(ctx, `SELECT value,revision FROM directory_classification WHERE singleton`).Scan(&raw, &revision); err != nil {
		return Record{}, err
	}
	return DecodeRecord(raw, revision)
}

// Render is the one source of truth for cloud and STRM paths. The original
// downloader name is evidence only, never renamed or deleted here.
func Render(p Policy, i metadata.Item, source, movieTemplate, seriesTemplate, extension string, season int, episodes []int) (string, Match, error) {
	m := p.Match(i)
	h := releasename.Parse(path.Base(source))
	h.Title, h.Year, h.MediaType = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, i.Title)), i.Year, i.MediaType
	h.Extension = extension
	template := movieTemplate
	if i.MediaType == "series" {
		if len(episodes) == 0 {
			return "", m, errors.New("剧集整理需要已确认集数")
		}
		h.Season, h.Episode = season, episodes[0]
		tag, err := metadata.EpisodeTag(season, episodes)
		if err != nil {
			return "", m, err
		}
		template = seriesTemplate
		template = strings.ReplaceAll(template, "{season_episode}", tag)
		template = strings.ReplaceAll(template, "S{season}E{episode}", tag)
	}
	rendered, err := releasename.RenderPath(template, h, source)
	if err != nil {
		return "", m, err
	}
	return path.Join(m.Directory, rendered), m, nil
}

package catalog

import (
	"context"
	"encoding/json"
	"io"
	"media-tool/internal/modules/settings"
	deferutil "media-tool/internal/platform/cleanup"
	"net/url"
	"strconv"
	"strings"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Card struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Type     string  `json:"type"`
	Year     int     `json:"year,omitempty"`
	Image    string  `json:"image"`
	Progress float64 `json:"progress,omitempty"`
	Subtitle string  `json:"subtitle,omitempty"`
}
type Home struct {
	Users     []User `json:"users"`
	UserID    string `json:"userId"`
	Libraries []Card `json:"libraries"`
	Playing   []Card `json:"playing"`
	Resume    []Card `json:"resume"`
	Recent    []Card `json:"recent"`
	Latest    []Card `json:"latest"`
}
type embyCard struct {
	ID           string            `json:"Id"`
	Name         string            `json:"Name"`
	Type         string            `json:"Type"`
	Year         int               `json:"ProductionYear"`
	SeriesName   string            `json:"SeriesName"`
	Index        *int              `json:"IndexNumber"`
	ImageTags    map[string]string `json:"ImageTags"`
	BackdropTags []string          `json:"BackdropImageTags"`
	UserData     struct {
		PlayedPercentage float64 `json:"PlayedPercentage"`
	} `json:"UserData"`
}

func convertCard(row embyCard) (Card, error) {
	if !safeID.MatchString(row.ID) {
		return Card{}, ErrUnavailable
	}
	out := Card{ID: row.ID, Title: row.Name, Type: row.Type, Year: row.Year, Progress: min(float64(100), max(float64(0), row.UserData.PlayedPercentage))}
	if row.Type == "Episode" && row.SeriesName != "" {
		out.Title = row.SeriesName
		if row.Index != nil {
			out.Subtitle = "第 " + strconv.Itoa(*row.Index) + " 集"
		}
	}
	typ := ""
	if len(row.BackdropTags) > 0 {
		typ = "backdrop"
	} else if row.ImageTags["Thumb"] != "" {
		typ = "thumb"
	} else if row.ImageTags["Primary"] != "" {
		typ = "primary"
	}
	if typ != "" {
		out.Image = "/api/v1/media/library/emby/" + row.ID + "/image?type=" + typ
	}
	return out, nil
}
func (e Engine) readEmby(ctx context.Context, route string, query url.Values, out any) error {
	response, err := e.embyRequest(ctx, route, query)
	if err != nil {
		return err
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || json.Unmarshal(raw, out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (e Engine) Home(ctx context.Context, requestedUser string) (Home, error) {
	out := Home{Users: []User{}, Libraries: []Card{}, Playing: []Card{}, Resume: []Card{}, Recent: []Card{}, Latest: []Card{}}
	var users []struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	}
	if err := e.readEmby(ctx, "/Users", nil, &users); err != nil {
		return out, err
	}
	if len(users) > 100 {
		return out, ErrUnavailable
	}
	for _, u := range users {
		if !safeID.MatchString(u.ID) {
			return out, ErrUnavailable
		}
		out.Users = append(out.Users, User{ID: u.ID, Name: u.Name})
	}
	if len(out.Users) == 0 {
		return out, ErrUnavailable
	}
	if requestedUser == "" {
		prefs, err := (settings.Repository{DB: e.DB}).Get(ctx)
		if err != nil {
			return out, err
		}
		requestedUser = prefs.Value.EmbyUserID
	}
	if requestedUser == "" {
		requestedUser = out.Users[0].ID
	}
	found := false
	for _, u := range out.Users {
		if u.ID == requestedUser {
			found = true
		}
	}
	if !found {
		return out, ErrInvalid
	}
	out.UserID = requestedUser
	endpoints := []struct {
		path   string
		query  url.Values
		target *[]Card
		array  bool
	}{{"/Users/" + requestedUser + "/Views", nil, &out.Libraries, false}, {"/Users/" + requestedUser + "/Items/Resume", url.Values{"Limit": {"12"}, "MediaTypes": {"Video"}, "Fields": {"Overview"}}, &out.Resume, false}, {"/Users/" + requestedUser + "/Items", url.Values{"Limit": {"12"}, "Recursive": {"true"}, "IsPlayed": {"true"}, "IncludeItemTypes": {"Movie,Episode"}, "SortBy": {"DatePlayed"}, "SortOrder": {"Descending"}}, &out.Recent, false}, {"/Users/" + requestedUser + "/Items/Latest", url.Values{"Limit": {"16"}, "IncludeItemTypes": {"Movie,Episode"}}, &out.Latest, true}}
	for _, request := range endpoints {
		rows := []embyCard{}
		if request.array {
			if err := e.readEmby(ctx, request.path, request.query, &rows); err != nil {
				return out, err
			}
		} else {
			var body struct {
				Items []embyCard `json:"Items"`
			}
			if err := e.readEmby(ctx, request.path, request.query, &body); err != nil {
				return out, err
			}
			rows = body.Items
		}
		if len(rows) > 100 {
			return out, ErrUnavailable
		}
		for _, row := range rows {
			card, err := convertCard(row)
			if err != nil {
				return out, err
			}
			*request.target = append(*request.target, card)
		}
	}
	var sessions []struct {
		UserID string    `json:"UserId"`
		Now    *embyCard `json:"NowPlayingItem"`
	}
	if err := e.readEmby(ctx, "/Sessions", nil, &sessions); err == nil {
		if len(sessions) > 200 {
			return out, ErrUnavailable
		}
		seen := map[string]bool{}
		for _, session := range sessions {
			if session.Now == nil || session.UserID != requestedUser || seen[session.Now.ID] {
				continue
			}
			card, err := convertCard(*session.Now)
			if err != nil {
				return out, err
			}
			seen[card.ID] = true
			out.Playing = append(out.Playing, card)
		}
	}
	return out, nil
}
func (e Engine) ImageType(ctx context.Context, id, typ string) ([]byte, string, error) {
	if typ == "" || typ == "primary" {
		return e.Image(ctx, id)
	}
	if !safeID.MatchString(id) || typ != "thumb" && typ != "backdrop" {
		return nil, "", ErrInvalid
	}
	name := "Thumb"
	if typ == "backdrop" {
		name = "Backdrop/0"
	}
	response, err := e.embyRequest(ctx, "/Items/"+id+"/Images/"+name, url.Values{"maxWidth": {"720"}, "quality": {"85"}})
	if err != nil {
		return nil, "", err
	}
	defer deferutil.Close(response.Body, "Read/descriptor teardown only; primary operations handle their own I/O result")
	content := strings.ToLower(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if content != "image/jpeg" && content != "image/png" && content != "image/webp" {
		return nil, "", ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 {
		return nil, "", ErrUnavailable
	}
	return raw, content, nil
}
func (e Engine) Item(ctx context.Context, id string) (Item, error) {
	if !safeID.MatchString(id) {
		return Item{}, ErrInvalid
	}
	var row embyCard
	var raw map[string]any
	if err := e.readEmby(ctx, "/Items/"+id, url.Values{"Fields": {"Overview"}}, &raw); err != nil {
		return Item{}, err
	}
	b, _ := json.Marshal(raw)
	if json.Unmarshal(b, &row) != nil || row.ID != id {
		return Item{}, ErrUnavailable
	}
	card, err := convertCard(row)
	if err != nil {
		return Item{}, err
	}
	kind := "series"
	if row.Type == "Movie" {
		kind = "movie"
	}
	overview, _ := raw["Overview"].(string)
	return Item{ID: id, Source: "emby", Title: card.Title, Year: strconv.Itoa(card.Year), MediaType: kind, Overview: overview, Poster: card.Image}, nil
}

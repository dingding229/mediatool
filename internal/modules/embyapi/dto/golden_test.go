package dto_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"media-tool/internal/modules/embyapi/dto"
)

var update = flag.Bool("update", false, "regenerate synthetic Emby DTO golden fixtures")

func ptr[T any](value T) *T { return &value }

type fixture struct {
	name      string
	full, min any
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	stamp := dto.NewUTCDateTime(time.Date(2026, 10, 16, 8, 9, 10, 123456700, time.FixedZone("synthetic", 8*3600)))
	public := dto.PublicSystemInfo{Id: strings.Repeat("a", 32), ServerName: "Synthetic Media", Version: "4.8.10.0", ProductName: "Emby Server", LocalAddress: "https://media.synthetic.invalid", OperatingSystem: "Linux", StartupWizardCompleted: true}
	policy := dto.UserPolicy{EnableMediaPlayback: true, EnableContentDownloading: true, EnabledFolders: dto.Array[string]{"100000"}, MaxParentalRating: ptr(0), SimultaneousStreamLimit: ptr(2)}
	configuration := dto.UserConfiguration{AudioLanguagePreference: "zho", SubtitleLanguagePreference: "eng", SubtitleMode: "Default", PlayDefaultAudioTrack: true, EnableNextEpisodeAutoPlay: true}
	user := dto.UserDto{Id: strings.Repeat("b", 32), Name: "Synthetic User", ServerId: public.Id, HasPassword: true, HasConfiguredPassword: true, LastLoginDate: &stamp, LastActivityDate: &stamp, Policy: policy, Configuration: configuration}
	pair := dto.NameIdPair{Name: "Synthetic Actor", Id: "900001"}
	person := dto.BaseItemPerson{NameIdPair: pair, Type: "Actor", Role: "Synthetic lead", PrimaryImageTag: "synthetic-person-tag"}
	stream := dto.MediaStream{Type: "Video", Codec: "h264", Language: "eng", DisplayTitle: "Synthetic 1080p", Index: ptr(0), BitRate: ptr(int64(0)), Width: ptr(1920), Height: ptr(1080), Channels: ptr(2), SampleRate: ptr(48000), BitDepth: ptr(8), AverageFrameRate: ptr(float32(24)), IsDefault: true, IsForced: true, IsExternal: true}
	chapter := dto.ChapterInfo{StartPositionTicks: 0, Name: "Synthetic opening", ImageTag: "synthetic-chapter-tag"}
	source, err := dto.NewMediaSourceInfo("https://media.synthetic.invalid/Videos/100005/stream.mkv?Static=true&MediaSourceId=source-0001")
	if err != nil {
		t.Fatal(err)
	}
	source.Id = "source-0001"
	source.Name = "Synthetic edition"
	source.Container = "mkv"
	source.RunTimeTicks = ptr(int64(72000000000))
	source.Size = ptr(int64(0))
	source.Bitrate = ptr(int64(5000000))
	source.DefaultAudioStreamIndex = ptr(0)
	source.DefaultSubtitleStreamIndex = ptr(-1)
	source.MediaStreams = dto.Array[dto.MediaStream]{stream}
	source.Chapters = dto.Array[dto.ChapterInfo]{chapter}
	sourceMin, err := dto.NewMediaSourceInfo("")
	if err != nil {
		t.Fatal(err)
	}
	data := dto.UserItemDataDto{PlaybackPositionTicks: ptr(int64(0)), PlayCount: ptr(0), Played: true, IsFavorite: true, LastPlayedDate: &stamp, UnplayedItemCount: ptr(0)}
	item, err := dto.NewBaseItemDto("100005", "Synthetic Episode", dto.Episode)
	if err != nil {
		t.Fatal(err)
	}
	item.OriginalTitle = "Synthetic Original"
	item.SortName = "Synthetic Episode"
	item.ServerId = public.Id
	item.ParentId = "100004"
	item.SeriesId = "100003"
	item.SeasonId = "100004"
	item.SeriesName = "Synthetic Series"
	item.CollectionType = "tvshows"
	item.Path = "Synthetic Episode.mkv"
	item.Overview = "Entirely synthetic contract metadata."
	item.RunTimeTicks = ptr(int64(72000000000))
	item.IndexNumber = ptr(0)
	item.IndexNumberEnd = ptr(0)
	item.ParentIndexNumber = ptr(0)
	item.ChildCount = ptr(0)
	item.RecursiveItemCount = ptr(0)
	item.ProductionYear = ptr(2026)
	item.CommunityRating = ptr(float32(8.5))
	item.OfficialRating = "PG"
	item.PremiereDate = &stamp
	item.DateCreated = &stamp
	item.Genres = dto.Array[string]{"Synthetic Drama"}
	item.People = dto.Array[dto.BaseItemPerson]{person}
	item.BackdropImageTags = dto.Array[string]{"synthetic-backdrop-tag"}
	item.ImageTags = map[string]string{"Primary": "synthetic-poster-tag"}
	item.ProviderIds = map[string]string{"Tmdb": "901001", "Imdb": "tt0000001"}
	item.MediaSources = dto.Array[dto.MediaSourceInfo]{source}
	item.MediaStreams = dto.Array[dto.MediaStream]{stream}
	item.Chapters = dto.Array[dto.ChapterInfo]{chapter}
	item.UserData = &data
	itemMin, err := dto.NewBaseItemDto("100000", "", dto.CollectionFolder)
	if err != nil {
		t.Fatal(err)
	}
	playState := dto.PlayStateInfo{PositionTicks: ptr(int64(0)), IsPaused: true, IsMuted: true, VolumeLevel: ptr(0), PlayMethod: "DirectPlay"}
	session := dto.SessionInfo{Id: "session-0001", UserId: user.Id, UserName: user.Name, DeviceId: "device-0001", DeviceName: "Synthetic TV", Client: "Synthetic Client", ApplicationVersion: "1.0.0", LastActivityDate: &stamp, NowPlayingItem: &item, PlayState: &playState}
	progress := dto.PlaybackProgressInfo{ItemId: item.Id, MediaSourceId: source.Id, PlaySessionId: "play-session-0001", SessionId: session.Id, PositionTicks: ptr(int64(9007199254740993)), RunTimeTicks: ptr(int64(9007199254741993)), AudioStreamIndex: ptr(0), SubtitleStreamIndex: ptr(-1), VolumeLevel: ptr(0), IsPaused: true, IsMuted: true, CanSeek: true, PlayMethod: "DirectPlay"}
	return []fixture{
		{"public_system_info", public, dto.PublicSystemInfo{}},
		{"system_info", dto.SystemInfo{PublicSystemInfo: public, HasUpdateAvailable: true, CanSelfRestart: true}, dto.SystemInfo{}},
		{"user_policy", policy, dto.UserPolicy{}},
		{"user_configuration", configuration, dto.UserConfiguration{}},
		{"user", user, dto.UserDto{}},
		{"authentication_result", dto.AuthenticationResult{User: user, SessionInfo: &session, AccessToken: "synthetic-user-token-not-a-real-secret", ServerId: public.Id}, dto.AuthenticationResult{}},
		{"name_id_pair", pair, dto.NameIdPair{}},
		{"person", person, dto.BaseItemPerson{}},
		{"base_item", item, itemMin},
		{"user_item_data", data, dto.UserItemDataDto{}},
		{"media_source", source, sourceMin},
		{"media_stream", stream, dto.MediaStream{}},
		{"chapter", chapter, dto.ChapterInfo{}},
		{"query_result", dto.QueryResult[dto.BaseItemDto]{Items: dto.Array[dto.BaseItemDto]{item}, TotalRecordCount: 1, StartIndex: ptr(0)}, dto.QueryResult[dto.BaseItemDto]{}},
		{"playback_info", dto.PlaybackInfoResponse{MediaSources: dto.Array[dto.MediaSourceInfo]{source}, PlaySessionId: progress.PlaySessionId, ErrorCode: "SyntheticCode"}, dto.PlaybackInfoResponse{}},
		{"play_state", playState, dto.PlayStateInfo{}},
		{"session", session, dto.SessionInfo{}},
		{"play_request", dto.PlayRequest(progress), dto.PlayRequest{}},
		{"playback_progress", progress, dto.PlaybackProgressInfo{}},
		{"playback_stop", dto.PlaybackStopInfo{ItemId: item.Id, MediaSourceId: source.Id, PlaySessionId: progress.PlaySessionId, SessionId: session.Id, PositionTicks: ptr(int64(9007199254740993)), Failed: true}, dto.PlaybackStopInfo{}},
	}
}
func compact(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func TestGoldens(t *testing.T) {
	for _, tc := range fixtures(t) {
		for _, variant := range []struct {
			name  string
			value any
		}{{"full", tc.full}, {"minimal", tc.min}} {
			t.Run(tc.name+"/"+variant.name, func(t *testing.T) {
				actual, err := json.MarshalIndent(variant.value, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				filename := filepath.Join("testdata", "emby", tc.name+"_"+variant.name+".json")
				if *update {
					if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filename, append(actual, '\n'), 0644); err != nil {
						t.Fatal(err)
					}
				}
				expected, err := os.ReadFile(filename)
				if err != nil {
					t.Fatal("missing golden (explicit -update required):", err)
				}
				if !bytes.Equal(compact(t, actual), compact(t, expected)) {
					t.Fatalf("wire contract differs from %s\ngot: %s\nwant: %s", filename, actual, expected)
				}
				inspect(t, expected)
				restored := reflect.New(reflect.TypeOf(variant.value)).Interface()
				if err := json.Unmarshal(expected, restored); err != nil {
					t.Fatal(err)
				}
				again, err := json.Marshal(restored)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(compact(t, again), compact(t, expected)) {
					t.Fatalf("round trip lost wire data: %s", again)
				}
			})
		}
	}
}

var arrayKeys = map[string]bool{"Items": true, "Genres": true, "People": true, "MediaSources": true, "MediaStreams": true, "BackdropImageTags": true, "Chapters": true, "EnabledFolders": true}

func inspect(t *testing.T, raw []byte) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		t.Fatal(err)
	}
	var visit func(any, string)
	visit = func(node any, path string) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				at := path + "." + key
				if key == "" || key[0] < 'A' || key[0] > 'Z' {
					t.Errorf("not PascalCase: %s", at)
				}
				if arrayKeys[key] {
					if _, ok := child.([]any); !ok {
						t.Errorf("array is null or not an array: %s (%T)", at, child)
					}
				}
				if key == "Id" || strings.HasSuffix(key, "Id") {
					if _, ok := child.(string); !ok {
						t.Errorf("ID is not a string: %s", at)
					}
				}
				if strings.HasSuffix(key, "Ticks") {
					number, ok := child.(json.Number)
					if !ok {
						t.Errorf("Ticks is not an integer: %s", at)
					} else if _, err := strconv.ParseInt(number.String(), 10, 64); err != nil {
						t.Errorf("Ticks outside int64: %s", at)
					}
				}
				if strings.HasSuffix(key, "Date") {
					text, ok := child.(string)
					if !ok {
						t.Errorf("date is not string: %s", at)
					} else if _, err := time.Parse("2006-01-02T15:04:05.0000000Z", text); err != nil {
						t.Errorf("not canonical UTC date: %s=%s", at, text)
					}
				}
				visit(child, at)
			}
		case []any:
			for i, child := range value {
				visit(child, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	visit(root, "$")
}

package dto

type PublicSystemInfo struct {
	Id                     string `json:"Id"`
	ServerName             string `json:"ServerName,omitempty"`
	Version                string `json:"Version,omitempty"`
	ProductName            string `json:"ProductName,omitempty"`
	LocalAddress           string `json:"LocalAddress,omitempty"`
	OperatingSystem        string `json:"OperatingSystem,omitempty"`
	StartupWizardCompleted bool   `json:"StartupWizardCompleted"`
}
type SystemInfo struct {
	PublicSystemInfo
	HasUpdateAvailable bool `json:"HasUpdateAvailable"`
	CanSelfRestart     bool `json:"CanSelfRestart"`
}
type UserPolicy struct {
	IsAdministrator          bool          `json:"IsAdministrator"`
	IsHidden                 bool          `json:"IsHidden"`
	IsDisabled               bool          `json:"IsDisabled"`
	EnableAllFolders         bool          `json:"EnableAllFolders"`
	EnableMediaPlayback      bool          `json:"EnableMediaPlayback"`
	EnableContentDownloading bool          `json:"EnableContentDownloading"`
	EnabledFolders           Array[string] `json:"EnabledFolders"`
	MaxParentalRating        *int          `json:"MaxParentalRating,omitempty"`
	SimultaneousStreamLimit  *int          `json:"SimultaneousStreamLimit,omitempty"`
}
type UserConfiguration struct {
	AudioLanguagePreference    string `json:"AudioLanguagePreference,omitempty"`
	SubtitleLanguagePreference string `json:"SubtitleLanguagePreference,omitempty"`
	SubtitleMode               string `json:"SubtitleMode,omitempty"`
	PlayDefaultAudioTrack      bool   `json:"PlayDefaultAudioTrack,omitempty"`
	EnableNextEpisodeAutoPlay  bool   `json:"EnableNextEpisodeAutoPlay,omitempty"`
}
type UserDto struct {
	Id                    string            `json:"Id"`
	Name                  string            `json:"Name,omitempty"`
	ServerId              string            `json:"ServerId,omitempty"`
	HasPassword           bool              `json:"HasPassword"`
	HasConfiguredPassword bool              `json:"HasConfiguredPassword"`
	LastLoginDate         *UTCDateTime      `json:"LastLoginDate,omitempty"`
	LastActivityDate      *UTCDateTime      `json:"LastActivityDate,omitempty"`
	Policy                UserPolicy        `json:"Policy"`
	Configuration         UserConfiguration `json:"Configuration"`
}
type AuthenticationResult struct {
	User        UserDto      `json:"User"`
	SessionInfo *SessionInfo `json:"SessionInfo,omitempty"`
	AccessToken string       `json:"AccessToken,omitempty"`
	ServerId    string       `json:"ServerId"`
}
type NameIdPair struct {
	Name string `json:"Name,omitempty"`
	Id   string `json:"Id"`
}
type UserItemDataDto struct {
	Key                   string       `json:"Key,omitempty"`
	PlayedPercentage      *float64     `json:"PlayedPercentage,omitempty"`
	PlaybackPositionTicks *int64       `json:"PlaybackPositionTicks,omitempty"`
	PlayCount             *int         `json:"PlayCount,omitempty"`
	Played                bool         `json:"Played"`
	IsFavorite            bool         `json:"IsFavorite"`
	LastPlayedDate        *UTCDateTime `json:"LastPlayedDate,omitempty"`
	UnplayedItemCount     *int         `json:"UnplayedItemCount,omitempty"`
}
type BaseItemPerson struct {
	NameIdPair
	Type            string `json:"Type,omitempty"`
	Role            string `json:"Role,omitempty"`
	PrimaryImageTag string `json:"PrimaryImageTag,omitempty"`
}
type BaseItemDto struct {
	Taglines                Array[string]          `json:"Taglines,omitempty"`
	Tags                    Array[string]          `json:"Tags,omitempty"`
	Id                      string                 `json:"Id"`
	Name                    string                 `json:"Name,omitempty"`
	OriginalTitle           string                 `json:"OriginalTitle,omitempty"`
	SortName                string                 `json:"SortName,omitempty"`
	ServerId                string                 `json:"ServerId,omitempty"`
	ParentId                string                 `json:"ParentId,omitempty"`
	SeriesId                string                 `json:"SeriesId,omitempty"`
	SeasonId                string                 `json:"SeasonId,omitempty"`
	SeriesName              string                 `json:"SeriesName,omitempty"`
	SeasonName              string                 `json:"SeasonName,omitempty"`
	Type                    ItemType               `json:"Type"`
	MediaType               string                 `json:"MediaType"`
	LocationType            string                 `json:"LocationType"`
	IsFolder                bool                   `json:"IsFolder"`
	CollectionType          string                 `json:"CollectionType,omitempty"`
	Path                    string                 `json:"Path,omitempty"`
	Overview                string                 `json:"Overview,omitempty"`
	RunTimeTicks            *int64                 `json:"RunTimeTicks,omitempty"`
	Container               string                 `json:"Container,omitempty"`
	Size                    *int64                 `json:"Size,omitempty"`
	Bitrate                 *int64                 `json:"Bitrate,omitempty"`
	Width                   *int                   `json:"Width,omitempty"`
	Height                  *int                   `json:"Height,omitempty"`
	IndexNumber             *int                   `json:"IndexNumber,omitempty"`
	IndexNumberEnd          *int                   `json:"IndexNumberEnd,omitempty"`
	ParentIndexNumber       *int                   `json:"ParentIndexNumber,omitempty"`
	ChildCount              *int                   `json:"ChildCount,omitempty"`
	RecursiveItemCount      *int                   `json:"RecursiveItemCount,omitempty"`
	ProductionYear          *int                   `json:"ProductionYear,omitempty"`
	CommunityRating         *float32               `json:"CommunityRating,omitempty"`
	OfficialRating          string                 `json:"OfficialRating,omitempty"`
	PremiereDate            *UTCDateTime           `json:"PremiereDate,omitempty"`
	EndDate                 *UTCDateTime           `json:"EndDate,omitempty"`
	Status                  string                 `json:"Status,omitempty"`
	SeriesPrimaryImageTag   string                 `json:"SeriesPrimaryImageTag,omitempty"`
	ParentLogoItemId        string                 `json:"ParentLogoItemId,omitempty"`
	ParentLogoImageTag      string                 `json:"ParentLogoImageTag,omitempty"`
	ParentThumbItemId       string                 `json:"ParentThumbItemId,omitempty"`
	ParentThumbImageTag     string                 `json:"ParentThumbImageTag,omitempty"`
	ParentBackdropItemId    string                 `json:"ParentBackdropItemId,omitempty"`
	ParentBackdropImageTags Array[string]          `json:"ParentBackdropImageTags"`
	DateCreated             *UTCDateTime           `json:"DateCreated,omitempty"`
	Genres                  Array[string]          `json:"Genres"`
	People                  Array[BaseItemPerson]  `json:"People"`
	BackdropImageTags       Array[string]          `json:"BackdropImageTags"`
	ImageTags               map[string]string      `json:"ImageTags,omitempty"`
	ProviderIds             map[string]string      `json:"ProviderIds,omitempty"`
	MediaSources            Array[MediaSourceInfo] `json:"MediaSources"`
	MediaStreams            Array[MediaStream]     `json:"MediaStreams"`
	Chapters                Array[ChapterInfo]     `json:"Chapters"`
	UserData                *UserItemDataDto       `json:"UserData,omitempty"`
}
type MediaStream struct {
	DeliveryUrl          string   `json:"DeliveryUrl,omitempty"`
	DeliveryMethod       string   `json:"DeliveryMethod,omitempty"`
	Profile              string   `json:"Profile,omitempty"`
	Level                *int     `json:"Level,omitempty"`
	RealFrameRate        *float32 `json:"RealFrameRate,omitempty"`
	PixelFormat          string   `json:"PixelFormat,omitempty"`
	AspectRatio          string   `json:"AspectRatio,omitempty"`
	VideoRange           string   `json:"VideoRange,omitempty"`
	VideoRangeType       string   `json:"VideoRangeType,omitempty"`
	IsInterlaced         bool     `json:"IsInterlaced,omitempty"`
	Rotation             *int     `json:"Rotation,omitempty"`
	ChannelLayout        string   `json:"ChannelLayout,omitempty"`
	Title                string   `json:"Title,omitempty"`
	IsTextSubtitleStream bool     `json:"IsTextSubtitleStream,omitempty"`
	IsHearingImpaired    bool     `json:"IsHearingImpaired,omitempty"`
	Type                 string   `json:"Type,omitempty"`
	Codec                string   `json:"Codec,omitempty"`
	Language             string   `json:"Language,omitempty"`
	DisplayTitle         string   `json:"DisplayTitle,omitempty"`
	Index                *int     `json:"Index,omitempty"`
	BitRate              *int64   `json:"BitRate,omitempty"`
	Width                *int     `json:"Width,omitempty"`
	Height               *int     `json:"Height,omitempty"`
	Channels             *int     `json:"Channels,omitempty"`
	SampleRate           *int     `json:"SampleRate,omitempty"`
	BitDepth             *int     `json:"BitDepth,omitempty"`
	AverageFrameRate     *float32 `json:"AverageFrameRate,omitempty"`
	IsDefault            bool     `json:"IsDefault,omitempty"`
	IsForced             bool     `json:"IsForced,omitempty"`
	IsExternal           bool     `json:"IsExternal,omitempty"`
}
type ChapterInfo struct {
	StartPositionTicks int64  `json:"StartPositionTicks"`
	Name               string `json:"Name,omitempty"`
	ImageTag           string `json:"ImageTag,omitempty"`
}
type QueryResult[T any] struct {
	Items            Array[T] `json:"Items"`
	TotalRecordCount int      `json:"TotalRecordCount"`
	StartIndex       *int     `json:"StartIndex,omitempty"`
}
type PlaybackInfoResponse struct {
	MediaSources  Array[MediaSourceInfo] `json:"MediaSources"`
	PlaySessionId string                 `json:"PlaySessionId,omitempty"`
	ErrorCode     string                 `json:"ErrorCode,omitempty"`
}
type PlayStateInfo struct {
	PositionTicks *int64 `json:"PositionTicks,omitempty"`
	IsPaused      bool   `json:"IsPaused,omitempty"`
	IsMuted       bool   `json:"IsMuted,omitempty"`
	VolumeLevel   *int   `json:"VolumeLevel,omitempty"`
	PlayMethod    string `json:"PlayMethod,omitempty"`
}
type SessionInfo struct {
	Id                 string         `json:"Id"`
	UserId             string         `json:"UserId,omitempty"`
	UserName           string         `json:"UserName,omitempty"`
	DeviceId           string         `json:"DeviceId,omitempty"`
	DeviceName         string         `json:"DeviceName,omitempty"`
	Client             string         `json:"Client,omitempty"`
	ApplicationVersion string         `json:"ApplicationVersion,omitempty"`
	LastActivityDate   *UTCDateTime   `json:"LastActivityDate,omitempty"`
	NowPlayingItem     *BaseItemDto   `json:"NowPlayingItem,omitempty"`
	PlayState          *PlayStateInfo `json:"PlayState,omitempty"`
}

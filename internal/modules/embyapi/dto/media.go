package dto

import "encoding/json"

type ItemType string

const (
	CollectionFolder ItemType = "CollectionFolder"
	Movie            ItemType = "Movie"
	Series           ItemType = "Series"
	Season           ItemType = "Season"
	Episode          ItemType = "Episode"
)

func (t ItemType) folder() bool { return t == CollectionFolder || t == Series || t == Season }
func NewBaseItemDto(id, name string, kind ItemType) (BaseItemDto, error) {
	item := BaseItemDto{Id: id, Name: name, Type: kind, MediaType: "Video", LocationType: "FileSystem", IsFolder: kind.folder()}
	return item, item.validate()
}
func (b BaseItemDto) validate() error {
	switch b.Type {
	case CollectionFolder, Movie, Series, Season, Episode:
	default:
		return ErrInvalid
	}
	if b.MediaType != "Video" || b.LocationType != "FileSystem" || b.IsFolder != b.Type.folder() || !safeFilename(b.Path) {
		return ErrInvalid
	}
	return safeMetadata(b)
}
func (b BaseItemDto) MarshalJSON() ([]byte, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	type wire BaseItemDto
	return json.Marshal(wire(b))
}
func (b *BaseItemDto) UnmarshalJSON(raw []byte) error {
	type wire BaseItemDto
	var decoded wire
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	value := BaseItemDto(decoded)
	if err := value.validate(); err != nil {
		return err
	}
	*b = value
	return nil
}

// Immutable capabilities: no writable Protocol/SupportsTranscoding/etc. fields.
// Even a zero value cannot advertise relay/transcoding. Path is an absolute
// user-scoped native stream URL, never a cloud path or global STRM resolver URL.
type MediaSourceInfo struct {
	Width                      *int               `json:"Width,omitempty"`
	Height                     *int               `json:"Height,omitempty"`
	Id                         string             `json:"Id,omitempty"`
	Name                       string             `json:"Name,omitempty"`
	Path                       string             `json:"Path,omitempty"`
	DirectStreamUrl            string             `json:"DirectStreamUrl,omitempty"`
	Container                  string             `json:"Container,omitempty"`
	RunTimeTicks               *int64             `json:"RunTimeTicks,omitempty"`
	Size                       *int64             `json:"Size,omitempty"`
	Bitrate                    *int64             `json:"Bitrate,omitempty"`
	DefaultAudioStreamIndex    *int               `json:"DefaultAudioStreamIndex,omitempty"`
	DefaultSubtitleStreamIndex *int               `json:"DefaultSubtitleStreamIndex,omitempty"`
	MediaStreams               Array[MediaStream] `json:"MediaStreams"`
	Chapters                   Array[ChapterInfo] `json:"Chapters"`
}

func NewMediaSourceInfo(playbackURL string) (MediaSourceInfo, error) {
	value := MediaSourceInfo{Path: playbackURL}
	if !safeStreamURL(playbackURL) {
		return MediaSourceInfo{}, ErrInvalid
	}
	return value, nil
}
func (MediaSourceInfo) Protocol() string           { return "Http" }
func (MediaSourceInfo) IsRemote() bool             { return true }
func (MediaSourceInfo) SupportsDirectPlay() bool   { return true }
func (MediaSourceInfo) SupportsDirectStream() bool { return true }
func (MediaSourceInfo) SupportsTranscoding() bool  { return false }
func (MediaSourceInfo) RequiresOpening() bool      { return false }
func (MediaSourceInfo) RequiresClosing() bool      { return false }

type sourceFields MediaSourceInfo
type sourceWire struct {
	sourceFields
	Protocol             string `json:"Protocol"`
	IsRemote             bool   `json:"IsRemote"`
	SupportsDirectPlay   bool   `json:"SupportsDirectPlay"`
	SupportsDirectStream bool   `json:"SupportsDirectStream"`
	SupportsTranscoding  bool   `json:"SupportsTranscoding"`
	RequiresOpening      bool   `json:"RequiresOpening"`
	RequiresClosing      bool   `json:"RequiresClosing"`
}

func (s MediaSourceInfo) MarshalJSON() ([]byte, error) {
	if !safeStreamURL(s.Path) || !safeDirectURL(s.Path, s.DirectStreamUrl) {
		return nil, ErrInvalid
	}
	if err := safeMetadata(s); err != nil {
		return nil, err
	}
	return json.Marshal(sourceWire{sourceFields(s), s.Protocol(), s.IsRemote(), s.SupportsDirectPlay(), s.SupportsDirectStream(), s.SupportsTranscoding(), s.RequiresOpening(), s.RequiresClosing()})
}
func (s *MediaSourceInfo) UnmarshalJSON(raw []byte) error {
	var decoded sourceFields
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	// Reject attempts to alter fixed capabilities, including case variants.
	var flags map[string]json.RawMessage
	if err := json.Unmarshal(raw, &flags); err != nil {
		return err
	}
	if err := validateCapabilities(flags); err != nil {
		return err
	}
	value := MediaSourceInfo(decoded)
	if !safeStreamURL(value.Path) || !safeDirectURL(value.Path, value.DirectStreamUrl) {
		return ErrInvalid
	}
	if err := safeMetadata(value); err != nil {
		return err
	}
	*s = value
	return nil
}

// Explicit false for embedded subtitles avoids client heuristics that infer
// external delivery from an absent property. Non-subtitle goldens stay intact.
func (s MediaStream) MarshalJSON() ([]byte, error) {
	if e := safeMetadata(s); e != nil {
		return nil, e
	}
	type fields MediaStream
	var external *bool
	if s.IsExternal || s.DeliveryMethod == "Embed" {
		v := s.IsExternal
		external = &v
	}
	return json.Marshal(struct {
		fields
		IsExternal *bool `json:"IsExternal,omitempty"`
	}{fields(s), external})
}

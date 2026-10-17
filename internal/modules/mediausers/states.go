package mediausers

type LegacyMediaType string

const (
	LegacyMovie  LegacyMediaType = "movie"
	LegacySeries LegacyMediaType = "series"
)

type PlaybackSessionState string

const (
	PlaybackPlaying PlaybackSessionState = "playing"
	PlaybackPaused  PlaybackSessionState = "paused"
	PlaybackStopped PlaybackSessionState = "stopped"
)

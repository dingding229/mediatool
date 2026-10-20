package mediausers

type PlaybackSessionState string

const (
	PlaybackPlaying PlaybackSessionState = "playing"
	PlaybackPaused  PlaybackSessionState = "paused"
	PlaybackStopped PlaybackSessionState = "stopped"
)

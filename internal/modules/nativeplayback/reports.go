package nativeplayback

import (
	"context"
	"time"
)

type ReportKind string

const (
	ReportPlaying  ReportKind = "playing"
	ReportProgress ReportKind = "progress"
	ReportStopped  ReportKind = "stopped"
	ReportPing     ReportKind = "ping"
)

// IDs and positions only; DeviceId/UserId/runtime supplied by clients are never trusted.
type Report struct {
	Kind                                 ReportKind
	ItemID, MediaSourceID, PlaySessionID string
	PositionTicks                        *int64
	IsPaused                             *bool
}
type Reporter interface {
	Report(context.Context, Actor, Report) error
}

// Resume policy uses the selected source's successful probe duration, not a
// client-supplied runtime. Unknown duration never marks an item watched.
func ViewingProgress(position, duration int64) (resume int64, completed bool) {
	if duration <= 0 {
		return position, false
	}
	fraction := float64(position) / float64(duration)
	if fraction > .90 {
		return 0, true
	}
	if fraction < .05 || duration < int64(5*time.Minute/(100*time.Nanosecond)) {
		return 0, false
	}
	return position, false
}

type ActiveSession struct {
	Type           string    `json:"type,omitempty"`
	Season         *int      `json:"season,omitempty"`
	Episode        *int      `json:"episode,omitempty"`
	EpisodeTitle   string    `json:"episodeTitle,omitempty"`
	ID             string    `json:"id"`
	UserID         string    `json:"userId"`
	UserName       string    `json:"userName"`
	DeviceID       string    `json:"deviceId"`
	DeviceName     string    `json:"deviceName"`
	ItemID         string    `json:"itemId"`
	ItemName       string    `json:"itemName"`
	State          string    `json:"state"`
	PositionTicks  int64     `json:"positionTicks"`
	LastProgressAt time.Time `json:"lastProgressAt"`
}
type ActiveSessions struct {
	Items      []ActiveSession `json:"items"`
	Total      int             `json:"total"`
	StartIndex int             `json:"startIndex"`
	Limit      int             `json:"limit"`
}

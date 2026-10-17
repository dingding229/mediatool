// Package admin exposes bounded administrator-only preprocessing controls.
// It never fetches media, returns raw probe payloads or accepts filesystem paths.
package admin

import (
	"errors"
	"media-tool/internal/modules/mediaprobe"
	"regexp"
	"strconv"
	"time"
)

var ErrInvalid = errors.New("invalid processing request")
var ErrConflict = errors.New("processing settings changed")

const MaxRetry = 500

type Settings struct {
	Enabled       bool  `json:"enabled"`
	ProbeEnabled  bool  `json:"probeEnabled"`
	StillsEnabled bool  `json:"stillsEnabled"`
	Revision      int64 `json:"revision"`
}
type StateCount struct {
	State string `json:"state"`
	Count int64  `json:"count"`
}
type Capacity struct {
	UsedBytes     int64 `json:"usedBytes"`
	ReservedBytes int64 `json:"reservedBytes"`
	LimitBytes    int64 `json:"limitBytes"`
	UnknownSizes  int64 `json:"unknownSizes"`
}
type Summary struct {
	Settings Settings     `json:"settings"`
	Probe    []StateCount `json:"probe"`
	Still    []StateCount `json:"still"`
	Capacity Capacity     `json:"capacity"`
}
type Filter struct {
	Kind, State    string
	Page, PageSize int
}
type Task struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	State         string     `json:"state"`
	ErrorReason   string     `json:"errorReason"`
	Attempts      int        `json:"attempts"`
	NextAttemptAt *time.Time `json:"nextAttemptAt"`
}
type Page struct {
	Items    []Task `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}
type Retry struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	State     string `json:"state"`
	Bulk      bool   `json:"bulk"`
	Confirmed bool   `json:"confirmed"`
}
type Retried struct {
	Selected     int  `json:"selected"`
	Queued       int  `json:"queued"`
	Skipped      int  `json:"skipped"`
	Failed       int  `json:"failed"`
	LimitReached bool `json:"limitReached"`
}

func States(kind string) []string {
	if kind == "probe" {
		return []string{string(mediaprobe.ProbePending), string(mediaprobe.ProbeProbing), string(mediaprobe.ProbeReady), string(mediaprobe.ProbeFailed), string(mediaprobe.ProbeNeedsAction)}
	}
	if kind == "still" {
		return []string{string(mediaprobe.StillPending), string(mediaprobe.StillGrabbing), string(mediaprobe.StillReady), string(mediaprobe.StillFailed), string(mediaprobe.StillNeedsAction), string(mediaprobe.StillSkipped)}
	}
	return nil
}
func (f *Filter) Valid() bool {
	states := States(f.Kind)
	if states == nil {
		return false
	}
	found := f.State == ""
	for _, s := range states {
		found = found || s == f.State
	}
	if !found {
		return false
	}
	if f.Page == 0 {
		f.Page = 1
	}
	if f.PageSize == 0 {
		f.PageSize = 50
	}
	return f.Page >= 1 && f.Page <= 100001 && f.PageSize >= 1 && f.PageSize <= 200 && (f.Page-1)*f.PageSize <= 1000000
}

var assetID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validTarget(kind, id string) bool {
	if kind == "probe" {
		return assetID.MatchString(id)
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return kind == "still" && err == nil && n >= 100000
}

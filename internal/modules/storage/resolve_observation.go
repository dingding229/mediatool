package storage

import (
	"context"
	"errors"
	"time"
)

var ErrRequestTimeout = errors.New("cloud request timed out")

type ResolveStage string

const (
	StageResolve              ResolveStage = "resolve"
	StageCacheVersion         ResolveStage = "cache_version"
	StageCacheLookup          ResolveStage = "cache_lookup"
	StageCacheHit             ResolveStage = "cache_hit"
	StageCacheMiss            ResolveStage = "cache_miss"
	StageCacheJoined          ResolveStage = "cache_joined"
	StageCacheWait            ResolveStage = "cache_wait"
	StageSourceQueue          ResolveStage = "source_queue"
	StageSourceResolve        ResolveStage = "source_resolve"
	StageCloudFile            ResolveStage = "cloud_file_lookup"
	StageCloudProfile         ResolveStage = "cloud_profile_lookup"
	StageOfficialToken        ResolveStage = "official_token"
	StageOfficialRefresh      ResolveStage = "official_refresh"
	StageOfficialPath         ResolveStage = "official_path_lookup"
	StageOfficialDownload     ResolveStage = "official_download"
	StageOfficialHTTPPath     ResolveStage = "official_http_path"
	StageOfficialHTTPDownload ResolveStage = "official_http_download"
	StageOfficialHTTPRefresh  ResolveStage = "official_http_refresh"
	StageFileIdentity         ResolveStage = "file_identity_check"
	StageDownloadIdentity     ResolveStage = "download_identity_check"
	StageTargetPolicy         ResolveStage = "target_policy"
	StageCDNCheck             ResolveStage = "cdn_check"
)

func (s ResolveStage) valid() bool {
	switch s {
	case StageResolve, StageCacheVersion, StageCacheLookup, StageCacheHit, StageCacheMiss, StageCacheWait, StageCacheJoined, StageSourceQueue, StageSourceResolve, StageCloudFile, StageCloudProfile, StageOfficialToken, StageOfficialRefresh, StageOfficialPath, StageOfficialDownload, StageOfficialHTTPPath, StageOfficialHTTPDownload, StageOfficialHTTPRefresh, StageFileIdentity, StageDownloadIdentity, StageTargetPolicy, StageCDNCheck:
		return true
	}
	return false
}

// Contains fixed enums/numbers only: never errors, URLs, paths, tokens, UA or IP.
// A value of -1 means the phase was not observed (e.g. a pooled connection).
// This is deliberately numeric/boolean only: never include headers or URLs.
type HTTPTraceTimings struct {
	Attempt          int   `json:"attempt"`
	DNSMillis        int64 `json:"dnsMs"`
	TCPMillis        int64 `json:"tcpMs"`
	TLSMillis        int64 `json:"tlsMs"`
	FirstByteMillis  int64 `json:"firstByteMs"`
	TotalMillis      int64 `json:"httpTotalMs"`
	ConnectionReused bool  `json:"connectionReused"`
	ResponseReceived bool  `json:"responseReceived"`
}

type ResolveObservation struct {
	Stage          ResolveStage
	Outcome        string
	DurationMillis int64
	ProviderCode   int64
	HTTPStatus     int
	HTTPTrace      *HTTPTraceTimings `json:"httpTrace,omitempty"`
}
type ResolveObserver func(ResolveObservation)
type resolveObserverKey struct{}

func WithResolveObserver(ctx context.Context, observer ResolveObserver) context.Context {
	return context.WithValue(ctx, resolveObserverKey{}, observer)
}

// Only explicitly sanitized provider metadata may cross the observation boundary.
type ProviderDiagnostic interface{ SafeDiagnostic() (int64, int) }

func ResolveFailure(err error) (kind string, code int64, status int) {
	if err == nil {
		return "ok", 0, 0
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled", 0, 0
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, ErrRequestTimeout):
		return "timeout", 0, 0
	case errors.Is(err, ErrUnauthenticated):
		return "authorization", 0, 0
	case errors.Is(err, ErrForbidden):
		return "forbidden", 0, 0
	case errors.Is(err, ErrInvalidResponse):
		return "invalid_response", 0, 0
	}
	var diagnostic ProviderDiagnostic
	if errors.As(err, &diagnostic) {
		code, status = diagnostic.SafeDiagnostic()
		if status < 100 || status > 599 {
			status = 0
		}
		if status == 429 || code == 40140117 {
			return "rate_limited", code, status
		}
		if status == 401 {
			return "authorization", code, status
		}
		return "provider_rejected", code, status
	}
	if errors.Is(err, ErrUnavailable) {
		return "unavailable", 0, 0
	}
	return "failed", 0, 0
}
func ObserveResolve(ctx context.Context, stage ResolveStage, started time.Time, err error) {
	ObserveResolveHTTP(ctx, stage, started, err, nil)
}

func ObserveResolveHTTP(ctx context.Context, stage ResolveStage, started time.Time, err error, timing *HTTPTraceTimings) {
	observer, _ := ctx.Value(resolveObserverKey{}).(ResolveObserver)
	if observer == nil || !stage.valid() {
		return
	}
	kind, code, status := ResolveFailure(err)
	observer(ResolveObservation{Stage: stage, Outcome: kind, DurationMillis: max(0, time.Since(started).Milliseconds()), ProviderCode: code, HTTPStatus: status, HTTPTrace: timing})
}

// SQL adapters may sanitize messages, but must not turn a cancelled context into
// a misleading 'file not found' or generic provider error.
func ContextFailure(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

package storage

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotImplemented  = errors.New("storage operation not implemented")
	ErrNotFound        = errors.New("cloud file not found")
	ErrUnauthenticated = errors.New("CloudDrive2 authentication failed")
	ErrForbidden       = errors.New("CloudDrive2 permission denied")
	ErrUnavailable     = errors.New("CloudDrive2 unavailable")
	ErrUnsupported     = errors.New("CloudDrive2 RPC unsupported by deployed version")
	ErrNotReady        = errors.New("CloudDrive2 is not logged in or ready")
	ErrNoDirectURL     = errors.New("CloudDrive2 did not return a direct cloud URL")
	ErrInvalidResponse = errors.New("CloudDrive2 returned an invalid response")
	ErrDirectory       = errors.New("cloud path refers to a directory")
	ErrLimitExceeded   = errors.New("directory exceeds the configured listing limit")
)

type FileRef struct {
	ExpectedSize int64  `json:"-"`
	UserAgent    string `json:"-"`
	ClientIP     string `json:"-"`
	AssetID      string `json:"assetId,omitempty"`
	Path         string `json:"path"`
	PickCode     string `json:"pickCode,omitempty"`
}
type File struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	IsDirectory bool              `json:"isDirectory"`
	Hashes      map[uint32]string `json:"hashes,omitempty"`
}
type DownloadLink struct {
	URL             string
	ExpiresAt       time.Time
	UserAgent       string
	Headers         map[string]string
	DirectOnly      bool      `json:"directOnly,omitempty"`
	CallerUserAgent bool      `json:"callerUserAgent,omitempty"`
	CacheUntil      time.Time `json:"cacheUntil,omitempty"`
}
type ProbeResult struct {
	SystemReady  bool `json:"systemReady"`
	LoggedIn     bool `json:"loggedIn"`
	PathReadable bool `json:"pathReadable"`
	File         File `json:"file"`
}
type PlaybackResolver interface {
	Resolve(context.Context, FileRef) (DownloadLink, error)
}
type Inspector interface {
	Probe(context.Context, string) (ProbeResult, error)
	Find(context.Context, string) (File, error)
}
type StorageProvider interface {
	PlaybackResolver
	Inspector
	List(context.Context, string) ([]File, error)
	Upload(context.Context, string, string) (string, error)
}

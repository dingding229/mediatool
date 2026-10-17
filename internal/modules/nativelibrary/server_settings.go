package nativelibrary

import (
	"context"
	"errors"
)

var ErrSettingsUnavailable = errors.New("native server settings unavailable")

// Public service identity only. No administrator or cloud credentials.
type ServerSettings struct {
	ServerID, ServerName, Version, PublicURL string
	Enabled, PublicUserList                  bool
	LibraryGranularity                       LibraryGranularity
	Revision                                 int64
}
type ServerSettingsReader interface {
	Get(context.Context) (ServerSettings, error)
}

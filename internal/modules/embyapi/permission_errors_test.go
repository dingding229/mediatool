package embyapi

import (
	"context"
	"encoding/json"
	"fmt"
	"media-tool/internal/modules/nativelibrary"
	"net/http/httptest"
	"strings"
	"testing"

	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativeplayback"
)

func TestPermissionErrorsAreOpaqueForBrowseAndReadableForPlay(t *testing.T) {
	for _, tc := range []struct {
		err     error
		message string
	}{{contracts.ErrForbidden, "权限不足"}, {contracts.ErrStreamLimit, "同时播放数"}} {
		wrapped := fmt.Errorf("synthetic-private-path SQL: %w", tc.err)
		w := httptest.NewRecorder()
		if err := WriteDomainError(w, wrapped); err != nil {
			t.Fatal(err)
		}
		if w.Code != 404 || strings.Contains(w.Body.String(), "synthetic") || strings.Contains(w.Body.String(), tc.message) {
			t.Fatal("browse existence leak", w.Code, w.Body.String())
		}
		w = httptest.NewRecorder()
		if err := WriteDomainError(w, nativeplayback.PermissionError(wrapped)); err != nil {
			t.Fatal(err)
		}
		if w.Code != 403 || !strings.Contains(w.Body.String(), tc.message) || strings.Contains(w.Body.String(), "SQL") {
			t.Fatal("playback denial mapping", w.Code, w.Body.String())
		}
	}
}

type permissionProbe struct {
	contracts.PermissionChecker
	marker int
}
type scopedAccount struct {
	*fakeAccountService
	seen []contracts.PermissionChecker
}

func (s *scopedAccount) GetUser(ctx context.Context, id string) (contracts.User, error) {
	info, ok := RequestInfo(ctx)
	if !ok || info.Permissions == nil {
		return contracts.User{}, contracts.ErrUnavailable
	}
	s.seen = append(s.seen, info.Permissions)
	return s.fakeAccountService.GetUser(ctx, id)
}
func TestPermissionFactoryIsPerRequestAndNeverSerialized(t *testing.T) {
	account := &scopedAccount{fakeAccountService: &fakeAccountService{user: contracts.User{ID: strings.Repeat("a", 32), Enabled: true, MaxStreams: 2}}}
	count := 0
	s, err := NewServer(ServerOptions{Settings: nativeSettingsFunc(func(context.Context) (nativelibrary.ServerSettings, error) { return settingsFixture(), nil }), Auth: account, Permissions: func() contracts.PermissionChecker {
		count++
		return &permissionProbe{PermissionChecker: contracts.NewPermissionChecker(), marker: count}
	}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("GET", "/Users/"+account.user.ID, nil)
		r.Header.Set("X-Emby-Token", "valid-token")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if count != 2 || len(account.seen) != 2 || account.seen[0] == account.seen[1] {
		t.Fatal("cross-request permission scope reused")
	}
	raw, err := json.Marshal(RequestContext{Permissions: account.seen[0]})
	if err != nil || string(raw) != "{}" {
		t.Fatal("permission context leaked", string(raw), err)
	}
}

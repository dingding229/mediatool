package nativepermissions

import (
	"context"
	"errors"
	"fmt"
	"media-tool/internal/modules/mediausers/contracts"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func checker(s snapshot) *Checker {
	c := New(nil)
	c.load = func(context.Context, contracts.User) (snapshot, error) { return s, nil }
	c.count = func(context.Context, contracts.User) (int, error) { return 0, nil }
	return c
}
func TestPermissionMatrix(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	expired := now.Add(-time.Second)
	for _, mode := range []string{"none", "browse", "play", "download", "invalid-download"} {
		for _, hidden := range []bool{false, true} {
			{
				for _, rating := range []string{"G", "R", ""} {
					for _, state := range []string{"enabled", "disabled", "expired", "must-change"} {
						name := fmt.Sprintf("%s/hidden=%v/rating=%s/%s", mode, hidden, rating, state)
						t.Run(name, func(t *testing.T) {
							u := contracts.User{ID: "user", Enabled: true, CredentialVersion: 1, MaxStreams: 2}
							stored := u
							switch state {
							case "disabled":
								stored.Enabled = false
							case "expired":
								stored.ExpiresAt = &expired
							case "must-change":
								stored.MustChangePassword = true
							}
							s := snapshot{User: stored}
							if mode != "none" && !hidden {
								s.Grants = []grant{{ID: "g:test", Play: mode == "play" || mode == "download", Download: mode == "download" || mode == "invalid-download"}}
							}
							c := checker(s)
							browse := mode != "none" && !hidden && state == "enabled"
							play := browse && (mode == "play" || mode == "download")
							download := play && mode == "download"
							item := browse
							for _, check := range []struct {
								name string
								err  error
								want bool
							}{{"browse", c.CanBrowse(ctx, u, "g:test"), browse}, {"play", c.CanPlay(ctx, u, "g:test"), play}, {"download", c.CanDownload(ctx, u, "g:test"), download}, {"item", c.ItemAllowed(ctx, u, contracts.ItemAccess{LibraryIDs: []string{"g:other", "g:test"}, OfficialRating: rating}), item}} {
								if (check.err == nil) != check.want {
									t.Fatalf("%s got %v", check.name, check.err)
								}
							}
							ids, err := c.VisibleLibraries(ctx, u)
							if state == "enabled" {
								if err != nil || len(ids) != boolInt(browse) {
									t.Fatal(ids, err)
								}
							} else if !errors.Is(err, contracts.ErrForbidden) {
								t.Fatal(err)
							}
						})
					}
				}
			}
		}
	}
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func TestRequestSnapshotOnceAndFreshRequestImmediatelySeesChange(t *testing.T) {
	u := contracts.User{ID: "user", Enabled: true, CredentialVersion: 1, MaxStreams: 2}
	ctx := context.Background()
	var loads atomic.Int32
	c := checker(snapshot{User: u})
	c.load = func(context.Context, contracts.User) (snapshot, error) {
		loads.Add(1)
		return snapshot{User: u, Grants: []grant{{ID: "g:test", Play: true}}}, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.CanPlay(ctx, u, "g:test"); err != nil {
				t.Error(err)
			}
			ids, err := c.VisibleLibraries(ctx, u)
			if err != nil || len(ids) != 1 {
				t.Error(ids, err)
			}
			ids[0] = "mutated"
		}()
	}
	wg.Wait()
	if loads.Load() != 1 {
		t.Fatal("more than one grant snapshot", loads.Load())
	}
	fresh := checker(snapshot{User: u})
	if err := fresh.CanBrowse(ctx, u, "g:test"); !errors.Is(err, contracts.ErrForbidden) {
		t.Fatal("cross-request cached authorization", err)
	}
	if err := c.ItemAllowed(ctx, u, contracts.ItemAccess{LibraryID: "g:absent"}); !errors.Is(err, contracts.ErrForbidden) {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Second)
	u.ExpiresAt = &expired
	if _, err := c.VisibleLibraries(ctx, u); !errors.Is(err, contracts.ErrForbidden) {
		t.Fatal("context account defense", err)
	}
}
func TestStreamSlotsAndLiveIdentity(t *testing.T) {
	u := contracts.User{ID: "u", Enabled: true, MaxStreams: 2}
	c := checker(snapshot{User: u})
	ctx := context.Background()
	for _, n := range []int{0, 1, 2, 3} {
		c.count = func(context.Context, contracts.User) (int, error) { return n, nil }
		out, err := c.StreamSlots(ctx, u)
		if out.Limit != 2 || out.InUse != n || (err == nil) != (n < 2) {
			t.Fatal(out, err)
		}
	}
	c.count = func(context.Context, contracts.User) (int, error) { return 0, contracts.ErrUnavailable }
	if _, err := c.StreamSlots(ctx, u); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
	c = New(nil)
	if _, err := c.VisibleLibraries(ctx, u); !errors.Is(err, contracts.ErrUnavailable) {
		t.Fatal(err)
	}
	c = checker(snapshot{User: u})
	u.CredentialVersion++
	if err := c.CanBrowse(ctx, u, "g:test"); !errors.Is(err, contracts.ErrForbidden) {
		t.Fatal("stale credentials accepted", err)
	}
}

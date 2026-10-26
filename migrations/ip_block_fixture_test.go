package migrations_test

import (
	"context"
	"media-tool/internal/modules/ipblock"
	"testing"
)

type emptyFixtureIPStore struct{}

func (emptyFixtureIPStore) Load(context.Context) (ipblock.Policy, error) {
	return ipblock.Policy{Revision: 1, Rules: []ipblock.Rule{}}, nil
}
func (emptyFixtureIPStore) Change(context.Context, ipblock.Mutation) (ipblock.Policy, error) {
	panic("versioned fixture IP policy must remain read-only")
}
func (emptyFixtureIPStore) AddHits(context.Context, string, map[string]ipblock.Delta) error {
	return nil
}

// Pre-86 schema fixtures deliberately test historical migrations. They carry no IP rules;
// the application still uses the real trust resolver and every existing auth/permission check.
func fixtureIPBlock(t *testing.T) *ipblock.Service {
	t.Helper()
	s := ipblock.New(emptyFixtureIPStore{}, nil)
	if e := s.Reload(t.Context()); e != nil {
		t.Fatal(e)
	}
	return s
}

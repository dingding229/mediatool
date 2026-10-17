package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"media-tool/internal/modules/jobs"
	"testing"
)

func TestChainRegistryPriorityScopeAndGeneration(t *testing.T) {
	s, first := activeFixture(t)
	ctx := context.Background()
	def, _ := definition(first.PluginID)
	second, err := s.Apply(ctx, "test-admin", Request{Kind: Install, OperationKey: jobs.UUID(), PluginID: def.Manifest.ID, Fingerprint: def.Fingerprint, Scopes: scopes(def), Config: json.RawMessage(`{}`), Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Apply(ctx, "test-admin", Request{Kind: Enable, OperationKey: jobs.UUID(), InstanceID: second.ID, ExpectedRevision: second.Revision, Fingerprint: def.Fingerprint, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	next, err := s.Claim(ctx, jobs.UUID())
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range []Lease{first, next} {
		d := contracts.Descriptor{Kind: "chain", ID: "name.recognize", Handler: contracts.HandlerID("chain", "name.recognize"), Priority: i * 10, RequiredScopes: []string{"events.chain.name.recognize"}}
		if err = s.Publish(ctx, l, contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{d}}); err != nil {
			t.Fatal(err)
		}
		if err = s.Renew(ctx, l, true); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.hooks(ctx, "chain", "name.recognize")
	if err != nil || len(list) != 2 || list[0].instance != next.ID {
		t.Fatal("priority", list, err)
	}
	if _, err = s.DB.Exec(ctx, `DELETE FROM plugin_v2_grants WHERE instance_id=$1 AND scope='events.chain.name.recognize'`, next.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Descriptor(ctx, next.ID, "chain", "name.recognize"); err == nil {
		t.Fatal("scope cached")
	}
	if _, err = s.DB.Exec(ctx, `UPDATE plugin_v2_instances SET grant_revision=grant_revision+1 WHERE id=$1`, next.ID); err != nil {
		t.Fatal(err)
	}
	list, err = s.hooks(ctx, "chain", "name.recognize")
	if err != nil || len(list) != 1 || list[0].instance != first.ID {
		t.Fatal("stale generation", list, err)
	}
}
func TestHookResultRejectsIdentityAndUnknownFields(t *testing.T) {
	for _, raw := range []string{`{"name":"x.mkv","hint":{"title":"x","mediaType":"movie"},"continue":true,"path":"/secret"}`, `{"name":"x.mkv","hint":{"title":"x","mediaType":"movie","tmdbId":42},"continue":true}`, `{"name":"x.mkv","hint":{"title":"x","mediaType":"movie","episodes":[-1]},"continue":true}`} {
		if validHookResult(json.RawMessage(raw)) {
			t.Fatal(raw)
		}
	}
}

package v2

import (
	"context"
	"encoding/json"
	"github.com/dingding229/mediatool-plugin/sdk/contracts"
	"testing"
	"time"
)

func TestDescriptorDisplayNameDoesNotMutateSignedMetadata(t *testing.T) {
	for _, tc := range []struct {
		d    contracts.Descriptor
		want string
	}{
		{contracts.Descriptor{ID: "assist", Name: "下载辅助检查"}, "下载辅助检查"},
		{contracts.Descriptor{ID: "downloader-status", Cmd: "downloader-status"}, "downloader status"},
		{contracts.Descriptor{ID: "some.service_id"}, "some service id"},
	} {
		snap := Snapshot{Metadata: contracts.Snapshot{Version: 2, Descriptors: []contracts.Descriptor{tc.d}}}
		raw, _ := json.Marshal(snap)
		if got := DescriptorName(tc.d); got != tc.want {
			t.Fatal(got)
		}
		after, _ := json.Marshal(snap)
		if string(raw) != string(after) {
			t.Fatal("display modified signed metadata")
		}
	}
}
func TestHistoryExcludesUIReadsBeforePagingAndRetainsAudit(t *testing.T) {
	s, l, snap := snapshotFixture(t)
	ctx := context.Background()
	var service contracts.Descriptor
	for _, d := range snap.Metadata.Descriptors {
		if d.Kind == "service" {
			service = d
			break
		}
	}
	business, err := s.Submit(ctx, snap, service, json.RawMessage(`{}`), "i44-history-business", "", nil, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	// Copy a valid audit row with kind=ui, as actual form/dashboard GET providers do.
	_, err = s.DB.Exec(ctx, `INSERT INTO plugin_v2_runs(id,instance_id,snapshot_id,generation,grant_revision,kind,handler,dedupe_key,request_hash,args_ciphertext,deadline_at) SELECT gen_random_uuid(),instance_id,snapshot_id,generation,grant_revision,'ui','ui.read', 'i44-ui-'||n,request_hash,args_ciphertext,deadline_at FROM plugin_v2_runs CROSS JOIN generate_series(1,4)n WHERE id=$1`, business)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.History(ctx, l.ID, 0, 1)
	if err != nil || len(got) != 1 || got[0].ID != business || got[0].OperationName != "tick" {
		t.Fatal(got, err)
	}
	next, err := s.History(ctx, l.ID, 1, 1)
	if err != nil || len(next) != 0 {
		t.Fatal(next, err)
	}
	var n int
	if err = s.DB.QueryRow(ctx, `SELECT count(*) FROM plugin_v2_runs WHERE instance_id=$1`, l.ID).Scan(&n); err != nil || n != 5 {
		t.Fatal("original audit lost", n, err)
	}
}
func TestRunNameUsesHistoricalDescriptorNotCurrentStatus(t *testing.T) {
	d := contracts.Descriptor{Kind: "action", ID: "assist", Name: "危险：按已确认配置运行（默认仅预览）", Handler: "action.test"}
	got := runOperationName(Run{Kind: RunAction, Handler: d.Handler}, contracts.Snapshot{Descriptors: []contracts.Descriptor{d}})
	if got != "按已确认配置运行（默认仅预览）" {
		t.Fatal(got)
	}
}

func TestPinnedManifestTitlesOverrideHistoricalFallback(t *testing.T) {
	def, err := definition("torrent-assistant")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"command", "service"} {
		id := "assist"
		if kind == "command" {
			id = "downloader-status"
		}
		d := contracts.Descriptor{Kind: kind, ID: id, Handler: contracts.HandlerID(kind, id), Name: "历史旧名称"}
		expected := def.Manifest.Extensions.Titles[kind+":"+id]
		if expected == "" {
			t.Fatal("plugin declaration lacks title", kind, id)
		}
		snap := contracts.Snapshot{Descriptors: []contracts.Descriptor{d}}
		if got := DescriptorName(d, def.Manifest.Extensions.Titles); got != expected {
			t.Fatal(got, expected)
		}
		if got := runOperationName(Run{Kind: RunKind(kind), Handler: d.Handler}, snap, def.Manifest.Extensions.Titles); got != expected {
			t.Fatal(got, expected)
		}
	}
}

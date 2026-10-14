package acquisition

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestVerifiedUploadCleanupDeletesOnlyItsOwnedFixture(t *testing.T) {
	f := newSourceFixture(t)
	ctx := f.ctx
	local := t.TempDir()
	name := "Owned.Movie.2026.mkv"
	data := []byte("owned-cleanup-fixture")
	h := sha1.Sum(data)
	sha := hex.EncodeToString(h[:])
	os.WriteFile(filepath.Join(local, name), data, 0600)
	os.WriteFile(filepath.Join(local, "unrelated.txt"), []byte("preserve"), 0600)
	var qbMu sync.Mutex
	present := true
	deletes := 0
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		qbMu.Lock()
		defer qbMu.Unlock()
		switch r.URL.Path {
		case "/api/v2/torrents/info":
			if !present {
				fmt.Fprint(w, "[]")
				return
			}
			json.NewEncoder(w).Encode([]map[string]any{{"hash": f.hash, "state": "stalledUP", "progress": 1, "save_path": "/downloads", "amount_left": 0, "completion_on": 1, "tags": f.taskID}})
		case "/api/v2/torrents/files":
			json.NewEncoder(w).Encode([]map[string]any{{"index": 0, "name": name, "size": len(data), "progress": 1, "priority": 1}})
		case "/api/v2/torrents/delete":
			r.ParseForm()
			if r.Form.Get("deleteFiles") != "true" || r.Form.Get("hashes") != f.hash {
				t.Error("unsafe deletion arguments")
			}
			deletes++
			os.Remove(filepath.Join(local, name))
			present = false
		default:
			w.WriteHeader(404)
		}
	}))
	defer qb.Close()
	qbRecord, err := f.engine.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "qbittorrent", Name: "Owned cleanup", Enabled: true, Settings: map[string]any{"endpoint": qb.URL, "downloadPath": "/downloads", "localPath": local}, Secrets: map[string]string{"username": "fixture", "password": "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	cloudPath := "/115/Owned/Owned.mkv"
	cloud := &pipelineCloud{files: map[string]*pb.CloudDriveFile{cloudPath: {Id: "cloud-owned", FullPathName: cloudPath, Name: "Owned.mkv", Size: int64(len(data)), FileType: pb.CloudDriveFile_File, FileHashes: map[uint32]string{2: sha}}}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(server, cloud)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()
	c, err := f.engine.Connections.Repository.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "Owned cloud", Enabled: true, Settings: map[string]any{"endpoint": "http://" + listener.Addr().String(), "cloudRoot": "/115/Owned"}, Secrets: map[string]string{"token": "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	file, media, transfer := jobs.UUID(), jobs.UUID(), jobs.UUID()
	raw := map[string]any{"endpoint": qb.URL, "downloadPath": "/downloads", "localPath": local, "category": "media-tool", "media": map[string]any{"id": 1, "title": "Owned Movie", "mediaType": "movie", "year": "2026"}}
	details, _ := json.Marshal(raw)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE download_tasks SET integration_id=$2,info_hash=$3,details=$4,download_state='imported'WHERE id=$1`, []any{f.taskID, qbRecord.ID, f.hash, details}},
		{`UPDATE resources SET public_metadata=public_metadata||'{"hr":false}'WHERE id=$1`, []any{f.resourceID}},
		{`INSERT INTO storage_connections(id,name,provider)VALUES($1,'Owned','clouddrive2')`, []any{c.ID}},
		{`INSERT INTO download_files(id,task_id,file_index,relative_path,size_bytes,local_path,media_state)VALUES($1,$2,0,$3,$4,$5,'imported')`, []any{file, f.taskID, name, len(data), filepath.Join(local, name)}},
		{`INSERT INTO transfer_tasks(id,download_file_id,integration_id,destination,device_id,source_sha1,state)VALUES($1,$2,$3,$4,'fixture',$5,'imported')`, []any{transfer, file, c.ID, cloudPath, sha}},
		{`INSERT INTO cloud_files(id,storage_id,provider_file_id,full_path,size_bytes)VALUES($1,$2,'cloud-owned',$3,$4)`, []any{file, c.ID, cloudPath, len(data)}},
		{`INSERT INTO media_assets(id,cloud_file_id)VALUES($1,$1)`, []any{file}},
		{`INSERT INTO media_items(id,media_type,tmdb_id,title,metadata)VALUES($1,'movie',1,'Owned Movie','{}')`, []any{media}},
		{`INSERT INTO media_asset_bindings(asset_id,media_id,download_file_id)VALUES($1,$2,$1)`, []any{file, media}},
	} {
		if _, err = f.engine.DB.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.engine.ApproveCleanup(ctx, f.taskID, true, true); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"taskId": f.taskID})
	if err = f.engine.Cleanup(ctx, jobs.Job{Payload: payload}); err != nil {
		t.Fatal(err)
	}
	qbMu.Lock()
	count := deletes
	qbMu.Unlock()
	if count != 1 {
		t.Fatal(count)
	}
	if _, err = os.Stat(filepath.Join(local, name)); !os.IsNotExist(err) {
		t.Fatal("owned media remained", err)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "unrelated.txt")); string(got) != "preserve" {
		t.Fatal("unrelated file deleted")
	}
	var state string
	f.engine.DB.QueryRow(ctx, `SELECT state FROM download_cleanup_requests WHERE task_id=$1`, f.taskID).Scan(&state)
	if state != "succeeded" {
		t.Fatal(state)
	}
	if err = f.engine.Cleanup(context.Background(), jobs.Job{Payload: payload}); err != nil {
		t.Fatal("deleted twice", err, deletes)
	}
	var cloudCount int
	f.engine.DB.QueryRow(ctx, `SELECT count(*)FROM cloud_files WHERE id=$1`, file).Scan(&cloudCount)
	if cloudCount != 1 || !strings.EqualFold(cloud.files[cloudPath].FileHashes[2], sha) {
		t.Fatal("cloud source changed")
	}
}

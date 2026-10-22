package acquisition

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"encoding/base64"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/downloads"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/modules/messaging"
	"media-tool/internal/modules/metadata"
	"media-tool/internal/modules/search"
	"media-tool/internal/modules/sites"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type pipelineCloud struct {
	pb.UnimplementedCloudDriveFileSrvServer
	mu          sync.Mutex
	files       map[string]*pb.CloudDriveFile
	destination string
	size        uint64
	data        []byte
	done        chan struct{}
}

func (c *pipelineCloud) FindFileByPath(ctx context.Context, r *pb.FindFileByPathRequest) (*pb.CloudDriveFile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	file := c.files[r.Path]
	if file == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	return file, nil
}
func (c *pipelineCloud) CreateFolder(ctx context.Context, r *pb.CreateFolderRequest) (*pb.CreateFolderResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	full := strings.TrimRight(r.ParentPath, "/") + "/" + r.FolderName
	file := &pb.CloudDriveFile{Id: full, Name: r.FolderName, FullPathName: full, IsDirectory: true}
	c.files[full] = file
	return &pb.CreateFolderResult{FolderCreated: file}, nil
}
func (c *pipelineCloud) StartRemoteUpload(ctx context.Context, r *pb.StartRemoteUploadRequest) (*pb.RemoteUploadStarted, error) {
	c.mu.Lock()
	c.destination = r.FilePath
	c.size = r.FileSize
	c.mu.Unlock()
	return &pb.RemoteUploadStarted{UploadId: "pipeline-upload"}, nil
}
func (c *pipelineCloud) RemoteUploadChannel(r *pb.RemoteUploadChannelRequest, stream grpc.ServerStreamingServer[pb.RemoteUploadChannelReply]) error {
	c.mu.Lock()
	size := c.size
	c.mu.Unlock()
	stream.Send(&pb.RemoteUploadChannelReply{UploadId: "pipeline-upload", Request: &pb.RemoteUploadChannelReply_ReadData{ReadData: &pb.RemoteReadDataRequest{Length: size}}})
	select {
	case <-c.done:
	case <-stream.Context().Done():
		return stream.Context().Err()
	}
	return stream.Send(&pb.RemoteUploadChannelReply{UploadId: "pipeline-upload", Request: &pb.RemoteUploadChannelReply_StatusChanged{StatusChanged: &pb.RemoteUploadStatusChanged{Status: pb.UploadFileInfo_Finish}}})
}
func (c *pipelineCloud) RemoteReadData(ctx context.Context, r *pb.RemoteReadDataUpload) (*pb.RemoteReadDataReply, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = append(c.data, r.Data...)
	if uint64(len(c.data)) == c.size {
		h := sha1.Sum(c.data)
		c.files[c.destination] = &pb.CloudDriveFile{Id: "uploaded-fixture", Name: filepath.Base(c.destination), FullPathName: c.destination, FileType: pb.CloudDriveFile_File, Size: int64(c.size), FileHashes: map[uint32]string{2: hex.EncodeToString(h[:])}}
		close(c.done)
	}
	return &pb.RemoteReadDataReply{Success: true, BytesReceived: uint64(len(r.Data))}, nil
}
func TestPostgresPipelineSearchDownloadUploadSTRMAndNotification(t *testing.T) {
	runFullPipeline(t, false)
}
func TestPostgresManualAdoptionReusesVerifiedUploadSTRMAndImport(t *testing.T) {
	runFullPipeline(t, true)
}
func runFullPipeline(t *testing.T, manual bool) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("real PostgreSQL test requires TEST_DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// This is a dedicated disposable test database, never the configured user DB.
	var databaseName string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "media_tool_test" {
		t.Fatal("pipeline test requires the isolated media_tool_test database")
	}
	db.Exec(ctx, `TRUNCATE notification_deliveries,notification_messages,outbox_events,jobs,media_asset_bindings,media_items,strm_entries,emby_item_mappings,media_assets,cloud_files,storage_connections,transfer_tasks,download_files,download_tasks,search_run_resources,resources,search_site_results,search_runs,integrations,site_definitions CASCADE`)
	defer db.Exec(context.Background(), `TRUNCATE notification_deliveries,notification_messages,outbox_events,jobs,media_asset_bindings,media_items,strm_entries,emby_item_mappings,media_assets,cloud_files,storage_connections,transfer_tasks,download_files,download_tasks,search_run_resources,resources,search_site_results,search_runs,integrations,site_definitions CASCADE`)
	local := t.TempDir()
	strmDir := t.TempDir()
	data := []byte("owned-test-video")
	os.WriteFile(filepath.Join(local, "Owned.Film.mkv"), data, 0600)
	torrent := []byte(fmt.Sprintf("d4:infod6:lengthi%de4:name14:Owned.Film.mkv12:piece lengthi16384e6:pieces20:%see", len(data), strings.Repeat("a", 20)))
	// name is 14 bytes including the dot before extension.
	added := false
	var qbmu sync.Mutex
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		qbmu.Lock()
		defer qbmu.Unlock()
		switch r.URL.Path {
		case "/api/v2/app/version":
			io.WriteString(w, "v5.0.4")
		case "/api/v2/torrents/add":
			if manual {
				t.Error("manual adoption re-added original torrent")
			}
			added = true
			io.WriteString(w, "Ok.")
		case "/api/v2/torrents/info":
			if !added {
				io.WriteString(w, "[]")
				return
			}
			fmt.Fprintf(w, `[{"hash":%q,"name":"Owned.Film.mkv","state":"stalledUP","progress":1,"save_path":"/downloads","content_path":"/downloads/Owned.Film.mkv","amount_left":0,"completion_on":1,"size":%d}]`, r.URL.Query().Get("hashes"), len(data))
		case "/api/v2/torrents/files":
			fmt.Fprintf(w, `[{"index":0,"name":"Owned.Film.mkv","progress":1,"priority":1,"size":%d}]`, len(data))
		default:
			w.WriteHeader(404)
		}
	}))
	defer qb.Close()
	pt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			io.WriteString(w, `<a href="logout.php">Logout</a>`)
		case "/torrents.php":
			fmt.Fprintf(w, `<table><tr class="resource"><td><a data-id="1">Owned Film</a><a class="download" href="/download.php?id=1&passkey=private-fixture">Download</a></td><td class="size">%d B</td><td class="seeds">10</td></tr></table>`, len(data))
		case "/download.php":
			w.Header().Set("Content-Type", "application/x-bittorrent")
			w.Write(torrent)
		default:
			w.WriteHeader(404)
		}
	}))
	defer pt.Close()
	emby := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "emby-token" {
			w.WriteHeader(403)
			return
		}
		if r.URL.Path == "/Library/Media/Updated" {
			var body struct{ Updates []struct{ Path string } }
			if json.NewDecoder(r.Body).Decode(&body) != nil || len(body.Updates) != 1 || !strings.HasSuffix(body.Updates[0].Path, ".strm") {
				t.Error("refresh lacks owned path evidence")
			}
			w.WriteHeader(204)
			return
		}
		if r.URL.Path == "/Items" {
			var asset string
			if err := db.QueryRow(r.Context(), `SELECT asset_id::text FROM strm_entries LIMIT 1`).Scan(&asset); err != nil {
				t.Error(err)
				w.WriteHeader(500)
				return
			}
			var relative string
			if err := db.QueryRow(r.Context(), `SELECT output_path FROM strm_entries WHERE asset_id=$1 LIMIT 1`, asset).Scan(&relative); err != nil {
				t.Fatal(err)
			}
			expected := path.Join("/strm", relative)
			json.NewEncoder(w).Encode(map[string]any{"Items": []map[string]any{{"Id": "owned-library-item", "Path": expected, "MediaSources": []map[string]string{{"Id": "owned-source", "Path": expected}}}}})
			return
		}

		w.WriteHeader(404)
	}))
	defer emby.Close()
	cloud := &pipelineCloud{files: map[string]*pb.CloudDriveFile{"/115/Media": {Id: "root", Name: "Media", FullPathName: "/115/Media", IsDirectory: true}}, done: make(chan struct{})}
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	grpcServer := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(grpcServer, cloud)
	go grpcServer.Serve(listener)
	defer grpcServer.Stop()
	defer listener.Close()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{PublicURL: "https://media.fixture.example", STRMKey: strings.Repeat("b", 64), STRMOutputDir: strmDir, STRMAllowedRoots: []string{"/115/Media"}, CloudDriveRequestTimeout: 5 * time.Second})
	defer manager.Close()
	manager.OutboundHTTP = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.themoviedb.org" {
			return http.DefaultTransport.RoundTrip(r)
		}
		if r.URL.Host != "api.themoviedb.org" || r.Header.Get("Authorization") != "Bearer tmdb-token" {
			t.Error("metadata credential or endpoint mismatch")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":99,"title":"Owned Film","release_date":"2026-01-01"}`))}, nil
	})}
	save := func(in integrations.Input) integrations.Record {
		t.Helper()
		record, err := repo.Save(ctx, "", in)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	account := save(integrations.Input{Kind: "ptsite", Name: "owned fixture site", Enabled: true, Settings: map[string]any{"definitionId": "owned_fixture", "endpoint": pt.URL}, Secrets: map[string]string{"cookie": "fixture=authorized"}})
	save(integrations.Input{Kind: "qbittorrent", Name: "owned fixture downloader", Enabled: true, Settings: map[string]any{"endpoint": qb.URL, "downloadPath": "/downloads", "localPath": local}})
	save(integrations.Input{Kind: "tmdb", Name: "owned fixture metadata", Enabled: true, Settings: map[string]any{"language": "zh-CN"}, Secrets: map[string]string{"token": "tmdb-token"}})
	save(integrations.Input{Kind: "clouddrive2", Name: "owned fixture cloud", Enabled: true, Settings: map[string]any{"endpoint": "http://" + listener.Addr().String(), "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "cloud-token"}})
	save(integrations.Input{Kind: "emby", Name: "owned fixture library", Enabled: true, Settings: map[string]any{"endpoint": emby.URL, "strmPathPrefix": "/strm"}, Secrets: map[string]string{"apiKey": "emby-token"}})
	definition := fmt.Sprintf(`{"templateVersion":1,"id":"owned_fixture","name":"Owned Fixture","schema":"NexusPHP","domain":%q,"requests":{"user_basic_info":{"parser":"XPath","method":"GET","path":"/","fields":{"is_login":{"selector":"//a[contains(@href,'logout')]","filters":[{"name":"not_blank"}]}}},"search":{"parser":"CssSelector","method":"GET","path":"/torrents.php","list":{"selector":"tr.resource"},"fields":{"id":{"selector":"a[data-id]","attribute":"data-id"},"title":{"selector":"a[data-id]"},"size":{"selector":"td.size","filters":[{"name":"byte_size"}]},"seeders":{"selector":"td.seeds"},"download":{"selector":"a.download","attribute":"href"}}}}}`, pt.URL)
	if _, err = (sites.Repository{DB: db}).Save(ctx, []byte(definition), []byte(`{}`), "owned-test-fixture"); err != nil {
		t.Fatal(err)
	}
	searchEngine := search.Engine{DB: db, Vault: vault, Connections: manager}
	runID, err := searchEngine.Start(ctx, "Owned Film", []string{account.ID})
	if err != nil {
		t.Fatal(err)
	}
	queue := jobs.Repository{DB: db}
	job, err := queue.Claim(ctx, "test-control", time.Minute)
	if err != nil || job == nil {
		t.Fatal(err)
	}
	if err = searchEngine.Process(ctx, *job); err != nil {
		t.Fatal(err)
	}
	queue.Finish(ctx, *job, nil)
	run, err := searchEngine.Get(ctx, runID, 1, 50)
	if err != nil || run.State != "partial" || len(run.Resources) != 1 || len(run.Sites) != 1 || run.Sites[0].State != "partial" || !strings.Contains(run.Sites[0].Message, "单页请求") {
		t.Fatal(err, run)
	}
	engine := Engine{DB: db, Connections: manager, Search: searchEngine}
	var taskID string
	if manual {
		taskID = jobs.UUID()
		var downloaderID string
		if err = db.QueryRow(ctx, `SELECT id::text FROM integrations WHERE kind='qbittorrent'`).Scan(&downloaderID); err != nil {
			t.Fatal(err)
		}
		sourceMeta, parseErr := downloads.ParseTorrent(torrent)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		options, sealErr := vault.Seal("manual-download:"+taskID, map[string]string{"autoImport": "true", "cleanupAfterImport": "false", "localPath": local, "torrent": base64.StdEncoding.EncodeToString(torrent), "url": ""})
		if sealErr != nil {
			t.Fatal(sealErr)
		}
		if _, err = db.Exec(ctx, `INSERT INTO manual_download_tasks(id,integration_id,info_hash,title,source_kind,source_label,payload_ciphertext,endpoint,save_path,category,state,submit_attempted)VALUES($1,$2,$3,'Owned.Film.2026.mkv','file','owned fixture',$4,$5,'/downloads','media-tool','completed',true)`, taskID, downloaderID, sourceMeta.InfoHash, options, qb.URL); err != nil {
			t.Fatal(err)
		}
		qbmu.Lock()
		added = true
		qbmu.Unlock()
		if err = engine.AdoptManual(ctx, taskID, metadata.Item{ID: 99, MediaType: "movie", Title: "Owned Film", Year: "2026"}); err != nil {
			t.Fatal(err)
		}
		if err = engine.AdoptManual(ctx, taskID, metadata.Item{ID: 99, MediaType: "movie", Title: "Owned Film", Year: "2026"}); err != nil {
			t.Fatal("manual adoption not idempotent", err)
		}
	} else {
		taskID, err = engine.Start(ctx, run.Resources[0].ID, "movie", 99)
		if err != nil {
			t.Fatal(err)
		}
		job, err = queue.Claim(ctx, "test-control", time.Minute)
		if err != nil || job == nil {
			t.Fatal(err)
		}
		if err = engine.Add(ctx, *job); err != nil {
			t.Fatal(err)
		}
		queue.Finish(ctx, *job, nil)
	}
	if err = engine.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, _, state, _, err := engine.load(ctx, taskID)
	if err != nil || state != "local_complete" {
		t.Fatal(state, err)
	}
	if err = engine.PrepareTransfers(ctx); err != nil {
		t.Fatal(err)
	}
	job, err = queue.ClaimQueue(ctx, "test-transfer", time.Minute, "transfer")
	if err != nil || job == nil {
		t.Fatal("transfer not queued", err)
	}

	var fencePayload struct {
		ID string `json:"transferId"`
	}
	json.Unmarshal(job.Payload, &fencePayload)
	release, err := engine.uploadFence(ctx, fencePayload.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	blocked, stop := context.WithTimeout(ctx, 80*time.Millisecond)
	concurrent, beginErr := db.Begin(blocked)
	if beginErr != nil {
		release()
		stop()
		t.Fatal(beginErr)
	}
	_, lockErr := concurrent.Exec(blocked, `SELECT pg_advisory_xact_lock(hashtextextended($1,714))`, fencePayload.ID)
	concurrent.Rollback(context.Background())
	stop()
	release()
	if lockErr == nil {
		t.Fatal("operator bypassed active RPC fence")
	}
	db.Exec(ctx, `UPDATE transfer_tasks SET state='paused',control_revision=2 WHERE id=$1`, fencePayload.ID)
	if release, err := engine.uploadFence(ctx, fencePayload.ID, 1); err == nil {
		release()
		t.Fatal("stale worker continued after pause")
	}
	db.Exec(ctx, `UPDATE transfer_tasks SET state='pending',control_revision=1 WHERE id=$1`, fencePayload.ID)
	if err = engine.Transfer(ctx, *job); err != nil {
		t.Fatal(err)
	}

	var actualBytes int64
	db.QueryRow(ctx, `SELECT bytes_transferred FROM transfer_tasks WHERE id=$1`, fencePayload.ID).Scan(&actualBytes)
	db.Exec(ctx, `UPDATE transfer_tasks SET transferred_ranges=transferred_ranges+int8multirange(int8range(0,$2,'[)'))WHERE id=$1`, fencePayload.ID, actualBytes)
	var dedupBytes int64
	db.QueryRow(ctx, `SELECT coalesce(sum(upper(r)-lower(r)),0)FROM transfer_tasks t CROSS JOIN LATERAL unnest(t.transferred_ranges)r WHERE t.id=$1`, fencePayload.ID).Scan(&dedupBytes)
	if actualBytes <= 0 || dedupBytes != actualBytes {
		t.Fatal("upload ranges not deduplicated", actualBytes, dedupBytes)
	}
	queue.Finish(ctx, *job, nil)
	job, err = queue.Claim(ctx, "test-control", time.Minute)
	if err != nil || job == nil || job.Kind != "emby.refresh" {
		t.Fatal("library refresh missing", err)
	}
	if err = engine.RefreshEmby(ctx, *job); err != nil {
		t.Fatal(err)
	}
	queue.Finish(ctx, *job, nil)
	job, err = queue.Claim(ctx, "test-control", time.Minute)
	if err != nil || job == nil || job.Kind != "emby.confirm" {
		t.Fatal("library confirmation missing", err)
	}
	if err = engine.ConfirmEmby(ctx, *job); err != nil {
		t.Fatal(err)
	}
	queue.Finish(ctx, *job, nil)
	var entries, assets int
	db.QueryRow(ctx, `SELECT count(*) FROM strm_entries`).Scan(&entries)
	db.QueryRow(ctx, `SELECT count(*) FROM media_assets`).Scan(&assets)
	if entries != 1 || assets != 1 {
		t.Fatal("media/STRM not indexed")
	}
	var output string
	db.QueryRow(ctx, `SELECT output_path FROM strm_entries`).Scan(&output)
	content, err := os.ReadFile(filepath.Join(strmDir, output))
	if err != nil || !strings.Contains(string(content), "/api/v1/cloudStorage/strm302?") {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(local, "Owned.Film.mkv")); err != nil || string(raw) != string(data) {
		t.Fatal("seeding source modified")
	}
	if err = engine.ImportFile(ctx, payloadTransferID(t, db)); err != nil {
		t.Fatal("repeat import not idempotent", err)
	}
	dispatcher := messaging.Dispatcher{DB: db, Connections: manager}
	if err = dispatcher.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	messages, err := dispatcher.List(ctx)
	if err != nil || len(messages) == 0 {
		t.Fatal("no durable notifications", err)
	}
	for _, message := range messages {
		if strings.Contains(message.Body, "passkey") || strings.Contains(message.Body, "cloud-token") {
			t.Fatal("notification exposed credentials")
		}
	}
	var taskList []Task
	taskList, err = engine.List(ctx)
	if err != nil || len(taskList) != 1 || taskList[0].State != "imported" {
		t.Fatal("pipeline did not reach current library refresh state", err, taskList)
	}
	raw, _ := json.Marshal(run)
	if strings.Contains(string(raw), "private-fixture") {
		t.Fatal("public search response exposed download hint")
	}
}
func payloadTransferID(t *testing.T, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), `SELECT id::text FROM transfer_tasks LIMIT 1`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

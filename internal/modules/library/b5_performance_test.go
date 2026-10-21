package library

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/integrations"
	"media-tool/internal/modules/jobs"
	"media-tool/internal/platform/config"
	"media-tool/internal/platform/database"
	"media-tool/internal/platform/secrets"
)

func TestB5ScanTenThousand(t *testing.T) {
	if os.Getenv("MEDIA_TOOL_B5_PERF") != "1" {
		t.Skip("explicit representative workload")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "media_tool_test" {
		t.Fatal("test database only", err)
	}
	cleanup := `TRUNCATE integrations,storage_connections,cloud_files,media_assets,jobs,outbox_events CASCADE`
	if _, err = db.Exec(ctx, cleanup); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), cleanup)
	cloud := &fixtureCloud{}
	for i := 0; i < 10000; i++ {
		cloud.files = append(cloud.files, &pb.CloudDriveFile{Id: fmt.Sprint("synthetic-", i), Name: fmt.Sprintf("Synthetic.%05d.mkv", i), FullPathName: fmt.Sprintf("/115/Media/Synthetic.%05d.mkv", i), FileType: pb.CloudDriveFile_File, Size: 100, FileHashes: map[uint32]string{2: fmt.Sprintf("%040x", i+1)}})
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := grpc.NewServer()
	pb.RegisterCloudDriveFileSrvServer(server, cloud)
	go server.Serve(listener)
	defer server.Stop()
	vault, _ := secrets.New(strings.Repeat("a", 64))
	repo := integrations.Repository{DB: db, Vault: vault}
	manager := integrations.NewManager(repo, config.Config{STRMAllowedRoots: []string{"/115/Media"}, CloudDriveRequestTimeout: 2 * time.Minute})
	defer manager.Close()
	_, err = repo.Save(ctx, "", integrations.Input{Kind: "clouddrive2", Name: "Synthetic B5 scan", Enabled: true, Settings: map[string]any{"endpoint": "http://" + listener.Addr().String(), "cloudRoot": "/115/Media"}, Secrets: map[string]string{"token": "synthetic-only"}})
	if err != nil {
		t.Fatal(err)
	}
	engine := Engine{DB: db, Connections: manager}
	id, err := engine.Start(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"scanId": id})
	dir := os.Getenv("B5_PROFILE_DIR")
	var cpu *os.File
	if dir != "" {
		os.MkdirAll(dir, 0700)
		cpu, err = os.Create(filepath.Join(dir, "scan.cpu.pprof"))
		if err != nil {
			t.Fatal(err)
		}
		if err = pprof.StartCPUProfile(cpu); err != nil {
			t.Fatal(err)
		}
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	err = engine.Process(ctx, jobs.Job{Payload: payload, Attempts: 1, MaxAttempts: 3})
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if cpu != nil {
		pprof.StopCPUProfile()
		cpu.Close()
		for _, kind := range []string{"heap", "allocs"} {
			f, e := os.Create(filepath.Join(dir, "scan."+kind+".pprof"))
			if e != nil {
				t.Fatal(e)
			}
			pprof.Lookup(kind).WriteTo(f, 0)
			f.Close()
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, `SELECT files_seen FROM library_scans WHERE id=$1`, id).Scan(&count); err != nil || count != 10000 {
		t.Fatal(count, err)
	}
	t.Logf("B5_SCAN ms=%.3f allocBytes=%d mallocs=%d files=%d", float64(elapsed)/1e6, after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs, count)
}

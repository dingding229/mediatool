package main

import (
	"context"
	"media-tool/internal/platform/database"
	"os"
	"testing"
	"time"
)

type maintenancePluginStub struct{ started, stopped chan struct{} }

func (s maintenancePluginStub) Run(ctx context.Context) error {
	s.started <- struct{}{}
	<-ctx.Done()
	s.stopped <- struct{}{}
	return nil
}
func TestPluginSupervisorDrainsAndResumesForMaintenance(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated postgres")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	db.QueryRow(ctx, `SELECT current_database()`).Scan(&name)
	if name != "media_tool_test" {
		t.Fatal(name)
	}
	db.Exec(ctx, `UPDATE ops_mode SET paused=false WHERE singleton`)
	defer db.Exec(context.Background(), `UPDATE ops_mode SET paused=false WHERE singleton`)
	s := maintenancePluginStub{make(chan struct{}, 4), make(chan struct{}, 4)}
	done := make(chan error, 1)
	run, stop := context.WithCancel(ctx)
	defer stop()
	go func() { done <- runPluginSupervisor(run, s, db) }()
	wait := func(ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-ctx.Done():
			t.Fatal("maintenance supervisor timeout")
		}
	}
	wait(s.started)
	db.Exec(ctx, `UPDATE ops_mode SET paused=true WHERE singleton`)
	wait(s.stopped)
	db.Exec(ctx, `UPDATE ops_mode SET paused=false WHERE singleton`)
	wait(s.started)
	stop()
	wait(s.stopped)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

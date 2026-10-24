package onlineupdate

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExecutionGateAndRecoveryLatchSurviveTaskRemoval(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("media_tool_test only")
	}
	base, _ := helperTestDB(t)
	id, _ := newID()
	schema := "update_control_" + id
	if _, e := base.Exec(t.Context(), `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	defer base.Exec(context.Background(), `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
	cfg, _ := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if _, e = db.Exec(t.Context(), `CREATE TABLE app_settings(LIKE public.app_settings INCLUDING ALL);CREATE TABLE audit_logs(LIKE public.audit_logs INCLUDING ALL)`); e != nil {
		t.Fatal(e)
	}
	s := &Service{Root: t.TempDir(), DB: db}
	c, e := s.Control(t.Context())
	if e != nil || c.Enabled || len(c.Recovery) != 0 {
		t.Fatal(e, c)
	}
	if _, e = s.Start(t.Context(), "admin", "0.28.5", "invalid"); !errors.Is(e, ErrDisabled) {
		t.Fatal("default gate failed", e)
	}
	if e = s.SetControl(t.Context(), "admin", "enable", ""); e != nil {
		t.Fatal(e)
	}
	j := Job{ID: id, Actor: "admin", Phase: "manual_recovery", CreatedAt: time.Now()}
	if e = save(s.Root, j); e != nil {
		t.Fatal(e)
	}
	if e = s.reconcileAudits(t.Context()); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(s.Root + "/" + id + ".json"); e != nil {
		t.Fatal(e)
	}
	c, e = s.Control(t.Context())
	if e != nil || len(c.Recovery) != 1 || c.Recovery[0] != id {
		t.Fatal("task deletion unlocked recovery", e, c)
	}
	if e = s.SetControl(t.Context(), "admin", "enable", ""); !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	if e = s.SetControl(t.Context(), "admin", "confirm_recovery", id); e != nil {
		t.Fatal(e)
	}
	// Reconciliation cannot relatch an explicitly confirmed task.
	if e = auditTerminal(t.Context(), db, j, "reconciliation"); e != nil {
		t.Fatal(e)
	}
	c, e = s.Control(t.Context())
	if e != nil || len(c.Recovery) != 0 {
		t.Fatal(e, c)
	}
	if e = s.SetControl(t.Context(), "admin", "confirm_recovery", id); e == nil {
		t.Fatal("duplicate confirmation accepted")
	}
	if e = s.SetControl(t.Context(), "admin", "disable", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Start(t.Context(), "admin", "0.28.5", "invalid"); !errors.Is(e, ErrDisabled) {
		t.Fatal(e)
	}
	var n int
	if e = db.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action='online_update.recovery_confirmed' AND details->>'id'=$1`, id).Scan(&n); e != nil || n != 1 {
		t.Fatal(e, n)
	}
}

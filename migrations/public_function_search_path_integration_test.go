package migrations_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"media-tool/internal/platform/buildinfo"
)

func TestPublicSQLAndPLpgSQLFunctionsHaveFixedSearchPath(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("isolated media_tool_test required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || cfg.ConnConfig.Database != "media_tool_test" {
		t.Fatal("isolated media_tool_test required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var actualDB string
	var version int64
	if err = pool.QueryRow(ctx, `SELECT current_database(), (SELECT max(version_id) FROM (SELECT DISTINCT ON(version_id) version_id,is_applied FROM public.goose_db_version ORDER BY version_id,id DESC) AS v WHERE is_applied)`).Scan(&actualDB, &version); err != nil || actualDB != "media_tool_test" || version != buildinfo.Schema {
		t.Fatal("run all test migrations before checking search_path", actualDB, version, err)
	}
	rows, err := pool.Query(ctx, `SELECT p.oid::regprocedure::text, EXISTS (
		SELECT 1 FROM unnest(p.proconfig) AS c(setting) WHERE split_part(c.setting, '=', 1)='search_path' AND split_part(c.setting, '=', 2)<>''
	) FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace
	JOIN pg_catalog.pg_language l ON l.oid=p.prolang
	WHERE n.nspname='public' AND l.lanname IN ('plpgsql','sql') AND p.prokind='f' ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var signature string
		var fixed bool
		if err := rows.Scan(&signature, &fixed); err != nil {
			t.Fatal(err)
		}
		count++
		if !fixed {
			t.Errorf("public function %s has no function-local search_path", signature)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no public application functions checked")
	}
	t.Logf("checked %d public SQL/PLpgSQL functions", count)
}

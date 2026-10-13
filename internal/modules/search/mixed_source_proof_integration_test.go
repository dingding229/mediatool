package search

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"media-tool/internal/modules/sites"
)

func TestPostgresMixedEmptyCollectionsCannotReplaceEarlierSourceProof(t *testing.T) {
	for _, change := range []string{"account_revision", "definition"} {
		t.Run(change, func(t *testing.T) {
			ctx, engine, account, newRun := snapshotSearchFixture(t)
			run := newRun()
			if _, err := engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id,state)VALUES($1,$2,'succeeded')`, run, account); err != nil {
				t.Fatal(err)
			}
			revision, fingerprint, err := sourceProof(ctx, engine.DB, account)
			if err != nil {
				t.Fatal(err)
			}
			client := sites.Client{SourceRevision: revision, DefinitionFingerprint: fingerprint}
			if err = engine.SaveResourcesWithClient(ctx, run, account, nil, client); err != nil {
				t.Fatal(err)
			}
			var original time.Time
			if err = engine.DB.QueryRow(ctx, `SELECT observed_at FROM search_site_results WHERE run_id=$1 AND account_id=$2`, run, account).Scan(&original); err != nil {
				t.Fatal(err)
			}
			if change == "account_revision" {
				_, err = engine.DB.Exec(ctx, `UPDATE integrations SET revision=revision+1 WHERE id=$1`, account)
			} else {
				_, err = engine.DB.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned_snapshot','Owned','NexusPHP','changed-after-empty-search','{}','{}','dingding229/mediatool-site')`)
			}
			if err != nil {
				t.Fatal(err)
			}
			newRevision, newFingerprint, err := sourceProof(ctx, engine.DB, account)
			if err != nil {
				t.Fatal(err)
			}
			// A later successful empty RSS read must not launder stale empty-search
			// evidence into the new origin, even when there are no resource rows.
			second := sites.Client{SourceRevision: newRevision, DefinitionFingerprint: newFingerprint}
			if err = engine.SaveResourcesWithClient(ctx, run, account, nil, second); !errors.Is(err, ErrSnapshotChanged) {
				t.Fatal("later collection replaced earlier source proof", err)
			}
			var retainedRevision int64
			var retainedFingerprint string
			var retainedObserved time.Time
			if err = engine.DB.QueryRow(ctx, `SELECT source_revision,definition_fingerprint,observed_at FROM search_site_results WHERE run_id=$1 AND account_id=$2`, run, account).Scan(&retainedRevision, &retainedFingerprint, &retainedObserved); err != nil {
				t.Fatal(err)
			}
			if retainedRevision != revision || retainedFingerprint != fingerprint || !retainedObserved.Equal(original) {
				t.Fatal("rejected mixed collection mutated stored proof")
			}
			if err = engine.ValidateRunSources(ctx, run); !errors.Is(err, ErrSnapshotChanged) {
				t.Fatal("stale mixed-source run passed validation", err)
			}
		})
	}
}

func TestPostgresMixedSourceKeepsFirstObservationEvenWithSearchWarnings(t *testing.T) {
	for _, state := range []string{"succeeded", "partial", "failed"} {
		t.Run(state, func(t *testing.T) {
			ctx, engine, account, newRun := snapshotSearchFixture(t)
			run := newRun()
			if _, err := engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id,state)VALUES($1,$2,$3)`, run, account, state); err != nil {
				t.Fatal(err)
			}
			revision, fingerprint, err := sourceProof(ctx, engine.DB, account)
			if err != nil {
				t.Fatal(err)
			}
			client := sites.Client{SourceRevision: revision, DefinitionFingerprint: fingerprint}
			if err = engine.SaveResourcesWithClient(ctx, run, account, nil, client); err != nil {
				t.Fatal(err)
			}
			var original time.Time
			if err = engine.DB.QueryRow(ctx, `UPDATE search_site_results SET observed_at=now()-interval '11 minutes' WHERE run_id=$1 AND account_id=$2 RETURNING observed_at`, run, account).Scan(&original); err != nil {
				t.Fatal(err)
			}
			if err = engine.SaveResourcesWithClient(ctx, run, account, nil, client); err != nil {
				t.Fatal(err)
			}
			var retained time.Time
			var retainedState string
			if err = engine.DB.QueryRow(ctx, `SELECT observed_at,state FROM search_site_results WHERE run_id=$1 AND account_id=$2`, run, account).Scan(&retained, &retainedState); err != nil || !retained.Equal(original) || retainedState != state {
				t.Fatal("mixed collection refreshed age or hid search warning", retainedState, err)
			}
			if err = engine.ValidateRunSources(ctx, run); !errors.Is(err, ErrSnapshotExpired) {
				t.Fatal("expired empty source escaped validation", err)
			}
		})
	}
}

func TestPostgresFailedSearchWithoutSuccessfulCollectionIsNotSourceProof(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	run := newRun()
	if _, err := engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id,state)VALUES($1,$2,'failed')`, run, account); err != nil {
		t.Fatal(err)
	}
	if err := engine.ValidateRunSources(ctx, run); err != nil {
		t.Fatal("failed HTTP collection invented successful evidence", err)
	}
	if _, err := engine.DB.Exec(ctx, `UPDATE search_site_results SET source_revision=1 WHERE run_id=$1 AND account_id=$2`, run, account); err != nil {
		t.Fatal(err)
	}
	if err := engine.ValidateRunSources(ctx, run); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatal("incomplete failed-state proof was silently ignored", err)
	}
}

func TestPostgresSourceProofCannotBeRecordedWithoutSiteResult(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	revision, fingerprint, err := sourceProof(ctx, engine.DB, account)
	if err != nil {
		t.Fatal(err)
	}
	run := newRun()
	if err = engine.SaveResourcesWithClient(ctx, run, account, []sites.Resource{{TorrentID: "42", Title: "Owned"}}, sites.Client{SourceRevision: revision, DefinitionFingerprint: fingerprint}); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatal("unrecorded source proof allowed a resource write", err)
	}
	var count int
	if err = engine.DB.QueryRow(ctx, `SELECT count(*) FROM resources`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unrecorded collection leaked global resources", count, err)
	}
}

func TestPostgresFutureDatedEmptySourceCannotPassValidation(t *testing.T) {
	ctx, engine, account, newRun := snapshotSearchFixture(t)
	run := newRun()
	revision, fingerprint, err := sourceProof(ctx, engine.DB, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id,state,source_revision,definition_fingerprint,observed_at)VALUES($1,$2,'failed',$3,$4,now()+interval '2 minutes')`, run, account, revision, fingerprint); err != nil {
		t.Fatal(err)
	}
	if err = engine.ValidateRunSources(ctx, run); !errors.Is(err, ErrSnapshotExpired) {
		t.Fatal("future-dated source proof passed validation", err)
	}
}

func TestPostgresUnprovenHTTPClientCannotWriteCollection(t *testing.T) {
	for _, client := range []sites.Client{{}, {SourceRevision: 1}, {DefinitionFingerprint: "unproven"}} {
		t.Run("incomplete_client", func(t *testing.T) {
			ctx, engine, account, newRun := snapshotSearchFixture(t)
			run := newRun()
			if _, err := engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id)VALUES($1,$2)`, run, account); err != nil {
				t.Fatal(err)
			}
			if err := engine.SaveResourcesWithClient(ctx, run, account, []sites.Resource{{TorrentID: "42", Title: "Owned"}}, client); !errors.Is(err, ErrSnapshotUnavailable) {
				t.Fatal("incomplete HTTP client bypassed source proof", err)
			}
			var count int
			if err := engine.DB.QueryRow(ctx, `SELECT count(*) FROM resources`).Scan(&count); err != nil || count != 0 {
				t.Fatal("incomplete HTTP proof published resource", count, err)
			}
		})
	}
}

func TestPostgresCommittingSourceValidationKeepsOriginAndCollectionLocks(t *testing.T) {
	for _, target := range []string{"account", "definition", "site_proof", "run"} {
		t.Run(target, func(t *testing.T) {
			ctx, engine, account, newRun := snapshotSearchFixture(t)
			run := newRun()
			if _, err := engine.DB.Exec(ctx, `INSERT INTO site_definitions(id,name,schema_name,source_commit,definition,common_definition,source_repository)VALUES('owned_snapshot','Owned','NexusPHP','fixture','{}','{}','dingding229/mediatool-site')`); err != nil {
				t.Fatal(err)
			}
			if _, err := engine.DB.Exec(ctx, `INSERT INTO search_site_results(run_id,account_id,state)VALUES($1,$2,'failed')`, run, account); err != nil {
				t.Fatal(err)
			}
			revision, fingerprint, err := sourceProof(ctx, engine.DB, account)
			if err != nil {
				t.Fatal(err)
			}
			if err = engine.SaveResourcesWithClient(ctx, run, account, nil, sites.Client{SourceRevision: revision, DefinitionFingerprint: fingerprint}); err != nil {
				t.Fatal(err)
			}
			tx, err := engine.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if err = engine.ValidateRunSourcesTx(ctx, tx, run); err != nil {
				t.Fatal(err)
			}
			contender, err := engine.DB.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer contender.Rollback(ctx)
			if _, err = contender.Exec(ctx, `SET LOCAL lock_timeout='75ms'`); err != nil {
				t.Fatal(err)
			}
			statement, id := "", run
			switch target {
			case "account":
				statement, id = `UPDATE integrations SET revision=revision+1 WHERE id=$1`, account
			case "definition":
				statement, id = `UPDATE site_definitions SET source_commit='changed' WHERE id=$1`, "owned_snapshot"
			case "site_proof":
				statement = `UPDATE search_site_results SET observed_at=now() WHERE run_id=$1`
			case "run":
				statement = `UPDATE search_runs SET keyword='changed' WHERE id=$1`
			}
			_, err = contender.Exec(ctx, statement, id)
			var pgerr *pgconn.PgError
			if !errors.As(err, &pgerr) || pgerr.Code != "55P03" {
				t.Fatal("committing proof did not retain lock", target, err)
			}
			if err = contender.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err = engine.DB.Exec(ctx, statement, id); err != nil {
				t.Fatal("source lock leaked beyond commit", target, err)
			}
		})
	}
}

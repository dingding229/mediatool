-- +goose Up
CREATE TABLE site_metric_schedule (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 enabled boolean NOT NULL DEFAULT true,
 interval_minutes integer NOT NULL DEFAULT 60 CHECK(interval_minutes BETWEEN 15 AND 10080),
 revision bigint NOT NULL DEFAULT 1,
 next_run_at timestamptz NOT NULL DEFAULT now()+interval '1 minute',
 last_dispatched_at timestamptz
);
INSERT INTO site_metric_schedule(singleton) VALUES(true);
CREATE INDEX jobs_site_metrics_active ON jobs((payload->>'accountId'))
 WHERE kind='site.metrics.refresh' AND state IN ('pending','running');
-- +goose Down
DROP INDEX jobs_site_metrics_active;
DROP TABLE site_metric_schedule;

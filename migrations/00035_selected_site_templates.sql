-- +goose Up
-- Keep existing definitions/snapshots. Only accounts' templates are implicitly selected.
CREATE TABLE site_template_loads (
    template_id text PRIMARY KEY,
    loaded_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO site_template_loads(template_id)
SELECT DISTINCT settings->>'definitionId' FROM integrations
WHERE kind='ptsite' AND settings->>'definitionId' IN ('hdsky','ourbits','ptchdbits','ssd','ttg','ubits');
-- +goose Down
DROP TABLE site_template_loads;

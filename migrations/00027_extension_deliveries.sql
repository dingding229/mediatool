-- +goose Up
CREATE TABLE extension_deliveries (
 event_id uuid NOT NULL,
 plugin_id text NOT NULL,
 state text NOT NULL CHECK(state IN('pending','succeeded','unknown')),
 message text NOT NULL DEFAULT '',
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(event_id,plugin_id)
);
-- +goose Down
DROP TABLE extension_deliveries;

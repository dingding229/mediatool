-- +goose Up
ALTER TABLE transfer_tasks ADD COLUMN endpoint_snapshot text;
UPDATE transfer_tasks t SET endpoint_snapshot=i.settings->>'endpoint' FROM integrations i WHERE i.id=t.integration_id;
-- +goose Down
ALTER TABLE transfer_tasks DROP COLUMN endpoint_snapshot;

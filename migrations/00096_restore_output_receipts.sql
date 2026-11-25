-- +goose Up
ALTER TABLE ops_web_restores ADD COLUMN output_receipt jsonb;
ALTER TABLE ops_requests ADD COLUMN output_receipt jsonb;

-- +goose Down
ALTER TABLE ops_requests DROP COLUMN output_receipt;
ALTER TABLE ops_web_restores DROP COLUMN output_receipt;

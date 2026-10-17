-- +goose Up
ALTER TABLE native_server_settings
 ADD COLUMN library_granularity text NOT NULL DEFAULT 'rule'
 CONSTRAINT native_server_settings_library_granularity_check CHECK (library_granularity IN ('rule','group'));

-- +goose Down
ALTER TABLE native_server_settings DROP COLUMN library_granularity;

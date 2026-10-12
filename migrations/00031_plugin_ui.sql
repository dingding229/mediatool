-- +goose Up
ALTER TABLE extension_plugins ADD COLUMN ui_html text NOT NULL DEFAULT '';
-- +goose Down
ALTER TABLE extension_plugins DROP COLUMN ui_html;

-- +goose Up
-- Reject incompatible rows; do not convert or erase existing data.
ALTER TABLE extension_plugins ADD CONSTRAINT extension_plugins_wasm_only CHECK
(coalesce(manifest->>'runtime'='wasm',false) AND coalesce(octet_length(wasm_module),0)>=8);
-- +goose Down
ALTER TABLE extension_plugins DROP CONSTRAINT extension_plugins_wasm_only;

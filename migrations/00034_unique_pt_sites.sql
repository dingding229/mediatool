-- +goose Up
-- Never delete existing duplicate credentials: a duplicate blocks migration.
CREATE UNIQUE INDEX integrations_builtin_ptsite_unique ON integrations((settings->>'definitionId')) WHERE kind='ptsite' AND settings->>'definitionId' IN ('hdsky','ourbits','ptchdbits','ssd','ttg','ubits');
-- +goose Down
DROP INDEX integrations_builtin_ptsite_unique;

-- +goose Up
ALTER TABLE plugin_v2_instances DROP CONSTRAINT plugin_v2_instances_reason_check;
ALTER TABLE plugin_v2_instances ADD CONSTRAINT plugin_v2_instances_reason_check CHECK(reason IN('','worker_failed','resource_limit','permission_review_required','definition_missing','definition_unsupported','data_unavailable','external_media_removed','sandbox_unavailable'));

-- +goose Down
UPDATE plugin_v2_instances SET reason='worker_failed' WHERE reason='sandbox_unavailable';
ALTER TABLE plugin_v2_instances DROP CONSTRAINT plugin_v2_instances_reason_check;
ALTER TABLE plugin_v2_instances ADD CONSTRAINT plugin_v2_instances_reason_check CHECK(reason IN('','worker_failed','resource_limit','permission_review_required','definition_missing','definition_unsupported','data_unavailable','external_media_removed'));

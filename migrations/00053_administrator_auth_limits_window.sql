-- +goose Up
CREATE INDEX administrator_auth_limits_window ON administrator_auth_limits(window_start);
-- +goose Down
DROP INDEX administrator_auth_limits_window;

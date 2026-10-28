-- +goose Up
-- Remove the user restriction only; do not update identities, grants or media.
ALTER TABLE media_users DROP COLUMN max_parental_rating;

-- +goose Down
-- Structure-only rollback: deleted limits cannot be reconstructed by Down.
-- Restoring old values requires the verified pre-upgrade database backup.
ALTER TABLE media_users ADD COLUMN max_parental_rating integer CHECK (max_parental_rating BETWEEN 0 AND 100);

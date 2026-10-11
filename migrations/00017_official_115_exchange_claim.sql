-- +goose Up
-- Claim the one-shot device code before calling 115. Persisting this state and
-- erasing the verifier prevents replay after process crashes or lost responses.
ALTER TABLE open115_authorizations DROP CONSTRAINT open115_authorizations_state_check;
ALTER TABLE open115_authorizations ADD CONSTRAINT open115_authorizations_state_check
 CHECK(state IN ('pending','exchanging','authorized','expired','cancelled','failed'));

-- +goose Down
-- A claim cannot safely return to pending: the upstream may have consumed it.
UPDATE open115_authorizations SET state='failed',payload_ciphertext='\x'::bytea WHERE state='exchanging';
ALTER TABLE open115_authorizations DROP CONSTRAINT open115_authorizations_state_check;
ALTER TABLE open115_authorizations ADD CONSTRAINT open115_authorizations_state_check
 CHECK(state IN ('pending','authorized','expired','cancelled','failed'));

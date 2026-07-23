BEGIN;

-- Rollback is intentionally explicit and ordered by foreign-key dependency.
-- Back up operational and audit data before applying this destructive down migration.
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS operation_items;
DROP TABLE IF EXISTS operations;
DROP TABLE IF EXISTS virtual_desktops;
DROP TABLE IF EXISTS thin_clients;
DROP TABLE IF EXISTS seats;
DROP TABLE IF EXISTS classrooms;
DROP TABLE IF EXISTS organizations;

COMMIT;

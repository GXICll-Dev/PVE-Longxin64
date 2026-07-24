BEGIN;

ALTER TABLE virtual_desktops
    ADD COLUMN baseline_snapshot_name text NOT NULL DEFAULT '';

ALTER TABLE operation_items
    ADD COLUMN snapshot_name text NOT NULL DEFAULT '';

COMMIT;

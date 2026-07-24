BEGIN;

-- Back up operation history before removing snapshot metadata.
ALTER TABLE operation_items
    DROP COLUMN snapshot_name;

ALTER TABLE virtual_desktops
    DROP COLUMN baseline_snapshot_name;

COMMIT;

BEGIN;

CREATE TABLE organizations (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE classrooms (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    name text NOT NULL,
    site text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('READY', 'ACTIVE', 'DEGRADED', 'OFFLINE')),
    timezone text NOT NULL DEFAULT 'Asia/Shanghai',
    template_name text NOT NULL DEFAULT '',
    template_version text NOT NULL DEFAULT '',
    active_session text,
    resource_version bigint NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE seats (
    id uuid PRIMARY KEY,
    classroom_id uuid NOT NULL REFERENCES classrooms(id) ON DELETE CASCADE,
    label text NOT NULL,
    operation_state text NOT NULL DEFAULT 'SUCCEEDED'
        CHECK (operation_state IN ('QUEUED', 'RUNNING', 'WAITING_PVE', 'VERIFYING', 'SUCCEEDED', 'FAILED', 'SKIPPED', 'UNKNOWN')),
    user_name text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (classroom_id, label)
);

CREATE TABLE thin_clients (
    id uuid PRIMARY KEY,
    seat_id uuid UNIQUE REFERENCES seats(id) ON DELETE SET NULL,
    device_uuid uuid NOT NULL UNIQUE,
    name text NOT NULL,
    online boolean NOT NULL DEFAULT false,
    ip_address text NOT NULL DEFAULT '',
    architecture text NOT NULL,
    agent_version text NOT NULL,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE virtual_desktops (
    id uuid PRIMARY KEY,
    seat_id uuid UNIQUE REFERENCES seats(id) ON DELETE SET NULL,
    cluster_id uuid NOT NULL,
    pve_vmid integer NOT NULL CHECK (pve_vmid > 0),
    name text NOT NULL,
    desired_state text NOT NULL CHECK (desired_state IN ('RUNNING', 'STOPPED', 'UNKNOWN')),
    observed_state text NOT NULL CHECK (observed_state IN ('RUNNING', 'STOPPED', 'UNKNOWN')),
    template_version text NOT NULL,
    guest_agent_ready boolean NOT NULL DEFAULT false,
    last_reconciled_at timestamptz,
    config_hash text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cluster_id, pve_vmid)
);

CREATE TABLE operations (
    id uuid PRIMARY KEY,
    classroom_id uuid NOT NULL REFERENCES classrooms(id),
    type text NOT NULL CHECK (type IN ('PRECHECK', 'START', 'SHUTDOWN', 'RESTORE')),
    status text NOT NULL CHECK (status IN (
        'QUEUED', 'VALIDATING', 'RUNNING', 'WAITING_PVE', 'VERIFYING',
        'SUCCEEDED', 'PARTIALLY_SUCCEEDED', 'FAILED', 'CANCEL_REQUESTED', 'CANCELLED'
    )),
    reason text NOT NULL DEFAULT '',
    request_id text NOT NULL,
    resource_version bigint NOT NULL DEFAULT 1 CHECK (resource_version > 0),
    lease_owner text,
    lease_until timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    started_at timestamptz,
    completed_at timestamptz
);

CREATE INDEX operations_status_created_at_idx ON operations (status, created_at);
CREATE INDEX operations_classroom_created_at_idx ON operations (classroom_id, created_at DESC);
CREATE INDEX operations_lease_idx ON operations (lease_until) WHERE completed_at IS NULL;

CREATE TABLE operation_items (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL REFERENCES operations(id) ON DELETE CASCADE,
    seat_id uuid NOT NULL REFERENCES seats(id),
    seat_label text NOT NULL,
    desktop_id uuid REFERENCES virtual_desktops(id) ON DELETE SET NULL,
    cluster_id uuid,
    pve_vmid integer CHECK (pve_vmid > 0),
    target_name text NOT NULL,
    status text NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'WAITING_PVE', 'VERIFYING', 'SUCCEEDED', 'FAILED', 'SKIPPED', 'UNKNOWN')),
    upid text NOT NULL DEFAULT '',
    error_code text NOT NULL DEFAULT '',
    message text NOT NULL DEFAULT '',
    started_at timestamptz,
    completed_at timestamptz,
    updated_at timestamptz NOT NULL,
    UNIQUE (operation_id, seat_id)
);

CREATE INDEX operation_items_operation_status_idx ON operation_items (operation_id, status);
CREATE INDEX operation_items_upid_idx ON operation_items (upid) WHERE upid <> '';

CREATE TABLE idempotency_keys (
    classroom_id uuid NOT NULL REFERENCES classrooms(id) ON DELETE CASCADE,
    key text NOT NULL CHECK (length(key) BETWEEN 1 AND 128),
    request_hash text NOT NULL,
    operation_id uuid NOT NULL UNIQUE REFERENCES operations(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (classroom_id, key)
);

CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    actor_id uuid,
    source_ip inet,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    reason text NOT NULL DEFAULT '',
    request_id text NOT NULL,
    operation_id uuid REFERENCES operations(id),
    result text NOT NULL,
    parameter_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_created_at_idx ON audit_events (created_at DESC);
CREATE INDEX audit_events_target_idx ON audit_events (target_type, target_id, created_at DESC);

COMMIT;

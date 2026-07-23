-- Local Compose seed only. Never use these identities as production data.
BEGIN;

INSERT INTO organizations (id, name)
VALUES ('00000000-0000-4000-8000-000000000001', '示范学校');

INSERT INTO classrooms (
    id, organization_id, name, site, status, timezone,
    template_name, template_version, active_session
) VALUES
(
    '10000000-0000-4000-8000-000000000001',
    '00000000-0000-4000-8000-000000000001',
    '龙芯云教室 A',
    '实训楼 3F',
    'ACTIVE',
    'Asia/Shanghai',
    'Windows 11 教学镜像',
    'v2026.07',
    '高一信息技术 · 第 3 节'
),
(
    '10000000-0000-4000-8000-000000000002',
    '00000000-0000-4000-8000-000000000001',
    '云教室 B',
    '实训楼 4F',
    'DEGRADED',
    'Asia/Shanghai',
    'Linux 编程基础',
    'v2026.06',
    NULL
);

INSERT INTO seats (id, classroom_id, label, user_name)
SELECT
    ('30000000-0000-4000-8001-' || lpad(seat_number::text, 12, '0'))::uuid,
    '10000000-0000-4000-8000-000000000001'::uuid,
    'A-' || lpad(seat_number::text, 2, '0'),
    CASE WHEN seat_number <= 10 THEN '学生 ' || lpad(seat_number::text, 2, '0') ELSE NULL END
FROM generate_series(1, 12) AS seat_number;

INSERT INTO seats (id, classroom_id, label)
SELECT
    ('30000000-0000-4000-8002-' || lpad(seat_number::text, 12, '0'))::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    'B-' || lpad(seat_number::text, 2, '0')
FROM generate_series(1, 8) AS seat_number;

INSERT INTO thin_clients (
    id, seat_id, device_uuid, name, online, ip_address,
    architecture, agent_version, last_seen_at
)
SELECT
    ('40000000-0000-4000-8001-' || lpad(seat_number::text, 12, '0'))::uuid,
    ('30000000-0000-4000-8001-' || lpad(seat_number::text, 12, '0'))::uuid,
    ('50000000-0000-4000-8001-' || lpad(seat_number::text, 12, '0'))::uuid,
    'thin-a-' || lpad(seat_number::text, 2, '0'),
    seat_number <> 12,
    '10.20.1.' || (100 + seat_number)::text,
    'loong64',
    '0.1.0-dev',
    now() - CASE WHEN seat_number = 12 THEN interval '20 minutes' ELSE interval '15 seconds' END
FROM generate_series(1, 12) AS seat_number;

INSERT INTO thin_clients (
    id, seat_id, device_uuid, name, online, ip_address,
    architecture, agent_version, last_seen_at
)
SELECT
    ('40000000-0000-4000-8002-' || lpad(seat_number::text, 12, '0'))::uuid,
    ('30000000-0000-4000-8002-' || lpad(seat_number::text, 12, '0'))::uuid,
    ('50000000-0000-4000-8002-' || lpad(seat_number::text, 12, '0'))::uuid,
    'thin-b-' || lpad(seat_number::text, 2, '0'),
    seat_number <= 6,
    '10.20.2.' || (100 + seat_number)::text,
    'loong64',
    '0.1.0-dev',
    now() - CASE WHEN seat_number <= 6 THEN interval '20 seconds' ELSE interval '30 minutes' END
FROM generate_series(1, 8) AS seat_number;

INSERT INTO virtual_desktops (
    id, seat_id, cluster_id, pve_vmid, name, desired_state,
    observed_state, template_version, guest_agent_ready,
    last_reconciled_at, config_hash
)
SELECT
    ('60000000-0000-4000-8001-' || lpad(seat_number::text, 12, '0'))::uuid,
    ('30000000-0000-4000-8001-' || lpad(seat_number::text, 12, '0'))::uuid,
    '70000000-0000-4000-8000-000000000001'::uuid,
    1000 + seat_number,
    'student-a-' || lpad(seat_number::text, 2, '0'),
    'RUNNING',
    CASE WHEN seat_number <= 10 THEN 'RUNNING' ELSE 'STOPPED' END,
    'v2026.07',
    seat_number <> 11,
    now() - interval '10 seconds',
    'demo-windows-v2026.07'
FROM generate_series(1, 12) AS seat_number;

INSERT INTO virtual_desktops (
    id, seat_id, cluster_id, pve_vmid, name, desired_state,
    observed_state, template_version, guest_agent_ready,
    last_reconciled_at, config_hash
)
SELECT
    ('60000000-0000-4000-8002-' || lpad(seat_number::text, 12, '0'))::uuid,
    ('30000000-0000-4000-8002-' || lpad(seat_number::text, 12, '0'))::uuid,
    '70000000-0000-4000-8000-000000000001'::uuid,
    1100 + seat_number,
    'student-b-' || lpad(seat_number::text, 2, '0'),
    'STOPPED',
    'STOPPED',
    'v2026.06',
    seat_number <= 6,
    now() - interval '20 seconds',
    'demo-linux-v2026.06'
FROM generate_series(1, 8) AS seat_number;

COMMIT;

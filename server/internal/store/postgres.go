package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(ctx context.Context, databaseURL string) (*PostgresRepository, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL configuration: %w", err)
	}
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	repository := &PostgresRepository{pool: pool}
	pingContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := repository.Ping(pingContext); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return repository, nil
}

func (repository *PostgresRepository) Ping(ctx context.Context) error {
	return repository.pool.Ping(ctx)
}

func (repository *PostgresRepository) Close() {
	repository.pool.Close()
}

func (repository *PostgresRepository) ListClassrooms(ctx context.Context) ([]domain.ClassroomSummary, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT c.id::text, c.name, c.site, c.status, c.timezone,
		       count(s.id)::int,
		       count(s.id) FILTER (
		         WHERE tc.online = true AND vd.guest_agent_ready = true
		           AND s.operation_state NOT IN ('FAILED', 'UNKNOWN')
		       )::int,
		       count(tc.id) FILTER (WHERE tc.online = true)::int,
		       count(vd.id) FILTER (WHERE vd.observed_state = 'RUNNING')::int,
		       c.template_name, c.template_version, c.active_session, c.updated_at
		FROM classrooms c
		LEFT JOIN seats s ON s.classroom_id = c.id
		LEFT JOIN thin_clients tc ON tc.seat_id = s.id
		LEFT JOIN virtual_desktops vd ON vd.seat_id = s.id
		GROUP BY c.id
		ORDER BY c.site, c.name`)
	if err != nil {
		return nil, fmt.Errorf("list classrooms: %w", err)
	}
	defer rows.Close()
	result := make([]domain.ClassroomSummary, 0)
	for rows.Next() {
		var summary domain.ClassroomSummary
		if err := rows.Scan(
			&summary.ID,
			&summary.Name,
			&summary.Site,
			&summary.Status,
			&summary.Timezone,
			&summary.SeatsTotal,
			&summary.SeatsReady,
			&summary.ThinClientsOnline,
			&summary.DesktopsRunning,
			&summary.TemplateName,
			&summary.TemplateVersion,
			&summary.ActiveSession,
			&summary.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan classroom summary: %w", err)
		}
		result = append(result, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate classrooms: %w", err)
	}
	return result, nil
}

func (repository *PostgresRepository) GetClassroom(ctx context.Context, id string) (domain.Classroom, error) {
	var classroom domain.Classroom
	err := repository.pool.QueryRow(ctx, `
		SELECT id::text, organization_id::text, name, site, status, timezone,
		       template_name, template_version, active_session, resource_version, updated_at
		FROM classrooms WHERE id = $1`, id).Scan(
		&classroom.ID,
		&classroom.OrganizationID,
		&classroom.Name,
		&classroom.Site,
		&classroom.Status,
		&classroom.Timezone,
		&classroom.TemplateName,
		&classroom.TemplateVersion,
		&classroom.ActiveSession,
		&classroom.ResourceVersion,
		&classroom.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Classroom{}, ErrNotFound
	}
	if err != nil {
		return domain.Classroom{}, fmt.Errorf("get classroom: %w", err)
	}

	rows, err := repository.pool.Query(ctx, `
		SELECT s.id::text, s.label, s.operation_state, s.user_name,
		       tc.id::text, tc.name, tc.online, tc.ip_address, tc.architecture,
		       tc.agent_version, tc.last_seen_at,
		       vd.id::text, vd.name, vd.cluster_id::text, vd.pve_vmid,
		       vd.desired_state, vd.observed_state, vd.template_version,
		       vd.baseline_snapshot_name, vd.guest_agent_ready, vd.last_reconciled_at, vd.config_hash
		FROM seats s
		LEFT JOIN thin_clients tc ON tc.seat_id = s.id
		LEFT JOIN virtual_desktops vd ON vd.seat_id = s.id
		WHERE s.classroom_id = $1
		ORDER BY s.label`, id)
	if err != nil {
		return domain.Classroom{}, fmt.Errorf("list classroom seats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var seat domain.Seat
		var terminalID, terminalName, terminalIP, terminalArchitecture, terminalVersion *string
		var terminalOnline *bool
		var terminalLastSeen *time.Time
		var desktopID, desktopName, desktopClusterID, desiredState, observedState, templateVersion, baselineSnapshot, configHash *string
		var desktopVMID *int
		var guestAgentReady *bool
		var lastReconciledAt *time.Time
		if err := rows.Scan(
			&seat.ID,
			&seat.Label,
			&seat.OperationState,
			&seat.UserName,
			&terminalID,
			&terminalName,
			&terminalOnline,
			&terminalIP,
			&terminalArchitecture,
			&terminalVersion,
			&terminalLastSeen,
			&desktopID,
			&desktopName,
			&desktopClusterID,
			&desktopVMID,
			&desiredState,
			&observedState,
			&templateVersion,
			&baselineSnapshot,
			&guestAgentReady,
			&lastReconciledAt,
			&configHash,
		); err != nil {
			return domain.Classroom{}, fmt.Errorf("scan classroom seat: %w", err)
		}
		if terminalID != nil {
			seat.Terminal = &domain.ThinClient{
				ID:           *terminalID,
				Name:         valueOrEmpty(terminalName),
				Online:       valueOrFalse(terminalOnline),
				IPAddress:    valueOrEmpty(terminalIP),
				Architecture: valueOrEmpty(terminalArchitecture),
				AgentVersion: valueOrEmpty(terminalVersion),
				LastSeenAt:   terminalLastSeen,
			}
		}
		if desktopID != nil {
			seat.Desktop = &domain.VirtualDesktop{
				ID:               *desktopID,
				Name:             valueOrEmpty(desktopName),
				ClusterID:        valueOrEmpty(desktopClusterID),
				PVEVMID:          valueOrZero(desktopVMID),
				DesiredState:     domain.PowerState(valueOrEmpty(desiredState)),
				ObservedState:    domain.PowerState(valueOrEmpty(observedState)),
				TemplateVersion:  valueOrEmpty(templateVersion),
				BaselineSnapshot: valueOrEmpty(baselineSnapshot),
				GuestAgentReady:  valueOrFalse(guestAgentReady),
				LastReconciledAt: lastReconciledAt,
				ConfigHash:       valueOrEmpty(configHash),
			}
		}
		classroom.Seats = append(classroom.Seats, seat)
	}
	if err := rows.Err(); err != nil {
		return domain.Classroom{}, fmt.Errorf("iterate classroom seats: %w", err)
	}
	return classroom, nil
}

func (repository *PostgresRepository) ListOperations(ctx context.Context, limit int) ([]domain.Operation, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var total int
	if err := repository.pool.QueryRow(ctx, `SELECT count(*)::int FROM operations`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count operations: %w", err)
	}
	rows, err := repository.pool.Query(ctx, `
		SELECT id::text FROM operations ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list operations: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("scan operation id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate operation ids: %w", err)
	}
	result := make([]domain.Operation, 0, len(ids))
	for _, id := range ids {
		operation, err := repository.GetOperation(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, operation)
	}
	return result, total, nil
}

func (repository *PostgresRepository) GetOperation(ctx context.Context, id string) (domain.Operation, error) {
	return getOperation(ctx, repository.pool, id)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func getOperation(ctx context.Context, querier rowQuerier, id string) (domain.Operation, error) {
	var operation domain.Operation
	err := querier.QueryRow(ctx, `
		SELECT o.id::text, o.classroom_id::text, c.name, o.type, o.status, o.reason,
		       o.request_id, o.resource_version, o.created_at, o.updated_at,
		       o.started_at, o.completed_at
		FROM operations o
		JOIN classrooms c ON c.id = o.classroom_id
		WHERE o.id = $1`, id).Scan(
		&operation.ID,
		&operation.ClassroomID,
		&operation.ClassroomName,
		&operation.Type,
		&operation.Status,
		&operation.Reason,
		&operation.RequestID,
		&operation.ResourceVersion,
		&operation.CreatedAt,
		&operation.UpdatedAt,
		&operation.StartedAt,
		&operation.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Operation{}, ErrNotFound
	}
	if err != nil {
		return domain.Operation{}, fmt.Errorf("get operation: %w", err)
	}
	rows, err := querier.Query(ctx, `
		SELECT id::text, operation_id::text, seat_id::text, seat_label, desktop_id::text,
		       cluster_id::text, pve_vmid, target_name, snapshot_name, status, upid, error_code, message,
		       started_at, completed_at, updated_at
		FROM operation_items WHERE operation_id = $1 ORDER BY seat_label, id`, id)
	if err != nil {
		return domain.Operation{}, fmt.Errorf("list operation items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.OperationItem
		var desktopID, clusterID *string
		var pveVMID *int
		if err := rows.Scan(
			&item.ID,
			&item.OperationID,
			&item.SeatID,
			&item.SeatLabel,
			&desktopID,
			&clusterID,
			&pveVMID,
			&item.TargetName,
			&item.SnapshotName,
			&item.Status,
			&item.UPID,
			&item.ErrorCode,
			&item.Message,
			&item.StartedAt,
			&item.CompletedAt,
			&item.UpdatedAt,
		); err != nil {
			return domain.Operation{}, fmt.Errorf("scan operation item: %w", err)
		}
		if desktopID != nil {
			item.DesktopID = *desktopID
		}
		if clusterID != nil {
			item.ClusterID = *clusterID
		}
		if pveVMID != nil {
			item.PVEVMID = *pveVMID
		}
		operation.Items = append(operation.Items, item)
	}
	if err := rows.Err(); err != nil {
		return domain.Operation{}, fmt.Errorf("iterate operation items: %w", err)
	}
	operation.RefreshCounts()
	return operation, nil
}

func (repository *PostgresRepository) CreateOperation(ctx context.Context, key, requestHash string, operation domain.Operation) (CreateOperationResult, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return CreateOperationResult{}, fmt.Errorf("begin operation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existingHash, existingID string
	err = tx.QueryRow(ctx, `
		SELECT request_hash, operation_id::text
		FROM idempotency_keys
		WHERE classroom_id = $1 AND key = $2`, operation.ClassroomID, key).Scan(&existingHash, &existingID)
	if err == nil {
		if existingHash != requestHash {
			return CreateOperationResult{}, ErrIdempotencyConflict
		}
		existing, err := getOperation(ctx, tx, existingID)
		if err != nil {
			return CreateOperationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CreateOperationResult{}, fmt.Errorf("commit idempotent lookup: %w", err)
		}
		return CreateOperationResult{Operation: existing, Created: false}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CreateOperationResult{}, fmt.Errorf("lookup idempotency key: %w", err)
	}

	operation.ResourceVersion = 1
	_, err = tx.Exec(ctx, `
		INSERT INTO operations (
		  id, classroom_id, type, status, reason, request_id, resource_version,
		  created_at, updated_at, started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		operation.ID,
		operation.ClassroomID,
		operation.Type,
		operation.Status,
		operation.Reason,
		operation.RequestID,
		operation.ResourceVersion,
		operation.CreatedAt,
		operation.UpdatedAt,
		operation.StartedAt,
		operation.CompletedAt,
	)
	if err != nil {
		return CreateOperationResult{}, fmt.Errorf("insert operation: %w", err)
	}
	for _, item := range operation.Items {
		var desktopID any
		if item.DesktopID != "" {
			desktopID = item.DesktopID
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO operation_items (
			  id, operation_id, seat_id, seat_label, desktop_id, cluster_id, pve_vmid, target_name, snapshot_name, status,
			  upid, error_code, message, started_at, completed_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
			item.ID,
			operation.ID,
			item.SeatID,
			item.SeatLabel,
			desktopID,
			nullableString(item.ClusterID),
			nullableInt(item.PVEVMID),
			item.TargetName,
			item.SnapshotName,
			item.Status,
			item.UPID,
			item.ErrorCode,
			item.Message,
			item.StartedAt,
			item.CompletedAt,
			item.UpdatedAt,
		)
		if err != nil {
			return CreateOperationResult{}, fmt.Errorf("insert operation item: %w", err)
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO idempotency_keys (classroom_id, key, request_hash, operation_id, created_at)
		VALUES ($1,$2,$3,$4,$5)`, operation.ClassroomID, key, requestHash, operation.ID, operation.CreatedAt)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			_ = tx.Rollback(ctx)
			return repository.resolveIdempotencyRace(ctx, operation.ClassroomID, key, requestHash)
		}
		return CreateOperationResult{}, fmt.Errorf("insert idempotency key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CreateOperationResult{}, fmt.Errorf("commit operation: %w", err)
	}
	operation.RefreshCounts()
	return CreateOperationResult{Operation: operation, Created: true}, nil
}

func (repository *PostgresRepository) resolveIdempotencyRace(ctx context.Context, classroomID, key, requestHash string) (CreateOperationResult, error) {
	var existingHash, operationID string
	err := repository.pool.QueryRow(ctx, `
		SELECT request_hash, operation_id::text FROM idempotency_keys
		WHERE classroom_id = $1 AND key = $2`, classroomID, key).Scan(&existingHash, &operationID)
	if err != nil {
		return CreateOperationResult{}, fmt.Errorf("resolve idempotency race: %w", err)
	}
	if existingHash != requestHash {
		return CreateOperationResult{}, ErrIdempotencyConflict
	}
	operation, err := repository.GetOperation(ctx, operationID)
	if err != nil {
		return CreateOperationResult{}, err
	}
	return CreateOperationResult{Operation: operation, Created: false}, nil
}

func (repository *PostgresRepository) SaveOperation(ctx context.Context, operation *domain.Operation) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save operation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	completed := operation.Status.Terminal()
	tag, err := tx.Exec(ctx, `
		UPDATE operations SET status=$1, reason=$2, updated_at=$3, started_at=$4,
		  completed_at=$5, resource_version=resource_version+1,
		  lease_owner=CASE WHEN $6 THEN NULL ELSE lease_owner END,
		  lease_until=CASE WHEN $6 THEN NULL ELSE lease_until END
		WHERE id=$7 AND resource_version=$8`,
		operation.Status,
		operation.Reason,
		operation.UpdatedAt,
		operation.StartedAt,
		operation.CompletedAt,
		completed,
		operation.ID,
		operation.ResourceVersion,
	)
	if err != nil {
		return fmt.Errorf("update operation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrVersionConflict
	}
	for _, item := range operation.Items {
		_, err = tx.Exec(ctx, `
			UPDATE operation_items SET status=$1, upid=$2, error_code=$3, message=$4,
			  started_at=$5, completed_at=$6, updated_at=$7
			WHERE id=$8 AND operation_id=$9`,
			item.Status,
			item.UPID,
			item.ErrorCode,
			item.Message,
			item.StartedAt,
			item.CompletedAt,
			item.UpdatedAt,
			item.ID,
			operation.ID,
		)
		if err != nil {
			return fmt.Errorf("update operation item: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit save operation: %w", err)
	}
	operation.ResourceVersion++
	operation.RefreshCounts()
	return nil
}

func (repository *PostgresRepository) ClaimNextOperation(ctx context.Context, owner string, leaseDuration time.Duration) (*domain.Operation, error) {
	var id string
	err := repository.pool.QueryRow(ctx, `
		WITH candidate AS (
		  SELECT id FROM operations
		  WHERE status NOT IN ('SUCCEEDED','PARTIALLY_SUCCEEDED','FAILED','CANCELLED','CANCEL_REQUESTED')
		    AND (lease_until IS NULL OR lease_until < now() OR lease_owner = $1)
		  ORDER BY created_at
		  FOR UPDATE SKIP LOCKED
		  LIMIT 1
		)
		UPDATE operations o
		SET lease_owner=$1, lease_until=now()+make_interval(secs => $2)
		FROM candidate WHERE o.id=candidate.id
		RETURNING o.id::text`, owner, leaseDuration.Seconds()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim operation: %w", err)
	}
	operation, err := repository.GetOperation(ctx, id)
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

func (repository *PostgresRepository) RenewOperationLease(ctx context.Context, operationID, owner string, leaseDuration time.Duration) error {
	tag, err := repository.pool.Exec(ctx, `
		UPDATE operations SET lease_until=now()+make_interval(secs => $1)
		WHERE id=$2 AND lease_owner=$3
		  AND status NOT IN ('SUCCEEDED','PARTIALLY_SUCCEEDED','FAILED','CANCELLED')`,
		leaseDuration.Seconds(), operationID, owner)
	if err != nil {
		return fmt.Errorf("renew operation lease: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrVersionConflict
	}
	return nil
}

func (repository *PostgresRepository) ApplyOperationItemResult(ctx context.Context, classroomID, seatID string, operationType domain.OperationType, status domain.ItemStatus) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin apply item result: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE seats SET operation_state=$1, updated_at=now()
		WHERE id=$2 AND classroom_id=$3`, status, seatID, classroomID)
	if err != nil {
		return fmt.Errorf("update seat operation state: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if status == domain.ItemSucceeded {
		switch operationType {
		case domain.OperationStart, domain.OperationRestore:
			_, err = tx.Exec(ctx, `
				UPDATE virtual_desktops SET desired_state='RUNNING', observed_state='RUNNING',
				  guest_agent_ready=true, last_reconciled_at=now(), updated_at=now()
				WHERE seat_id=$1`, seatID)
		case domain.OperationShutdown:
			_, err = tx.Exec(ctx, `
				UPDATE virtual_desktops SET desired_state='STOPPED', observed_state='STOPPED',
				  guest_agent_ready=false, last_reconciled_at=now(), updated_at=now()
				WHERE seat_id=$1`, seatID)
		}
		if err != nil {
			return fmt.Errorf("update desktop state: %w", err)
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE classrooms SET resource_version=resource_version+1, updated_at=now() WHERE id=$1`, classroomID)
	if err != nil {
		return fmt.Errorf("touch classroom: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit item result: %w", err)
	}
	return nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func valueOrFalse(value *bool) bool {
	return value != nil && *value
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

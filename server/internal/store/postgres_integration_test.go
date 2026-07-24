package store

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GXICll-Dev/PVE-Longxin64/server/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRepositoryOperationLifecycle(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("PVE_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("PVE_TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	schema := "pve_test_" + strings.ReplaceAll(domain.NewID(), "-", "")
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL test database: %v", err)
	}
	schemaCreated := false
	t.Cleanup(func() {
		if schemaCreated {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_, _ = adminPool.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		}
		adminPool.Close()
	})
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	schemaCreated = true

	scopedURL, err := databaseURLWithSearchPath(databaseURL, schema)
	if err != nil {
		t.Fatalf("scope PostgreSQL URL: %v", err)
	}
	applyTestMigration(t, ctx, scopedURL)
	seedPostgresRepository(t, ctx, scopedURL)

	var repository *PostgresRepository
	repository, err = NewPostgresRepository(ctx, scopedURL)
	if err != nil {
		t.Fatalf("open PostgreSQL repository: %v", err)
	}
	t.Cleanup(func() {
		if repository != nil {
			repository.Close()
		}
	})

	classrooms, err := repository.ListClassrooms(ctx)
	if err != nil {
		t.Fatalf("list classrooms: %v", err)
	}
	if len(classrooms) != 1 || classrooms[0].SeatsTotal != 1 {
		t.Fatalf("unexpected classroom summary: %+v", classrooms)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	operation := domain.Operation{
		ID:          postgresTestOperationID,
		ClassroomID: postgresTestClassroomID,
		Type:        domain.OperationRestore,
		Status:      domain.OperationQueued,
		RequestID:   "postgres-integration-request",
		CreatedAt:   now,
		UpdatedAt:   now,
		Items: []domain.OperationItem{{
			ID:           postgresTestItemID,
			OperationID:  postgresTestOperationID,
			SeatID:       postgresTestSeatID,
			SeatLabel:    "01",
			DesktopID:    postgresTestDesktopID,
			ClusterID:    postgresTestClusterID,
			PVEVMID:      101,
			TargetName:   "student-test-01",
			SnapshotName: "classroom-baseline",
			Status:       domain.ItemQueued,
			UpdatedAt:    now,
		}},
	}

	created, err := repository.CreateOperation(ctx, "stable-key", "request-hash", operation)
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if !created.Created || created.Operation.ID != operation.ID {
		t.Fatalf("unexpected create result: %+v", created)
	}

	replayed, err := repository.CreateOperation(ctx, "stable-key", "request-hash", operation)
	if err != nil {
		t.Fatalf("replay idempotent operation: %v", err)
	}
	if replayed.Created || replayed.Operation.ID != operation.ID {
		t.Fatalf("idempotent replay did not return the original operation: %+v", replayed)
	}
	if _, err := repository.CreateOperation(ctx, "stable-key", "different-hash", operation); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	claimed, err := repository.ClaimNextOperation(ctx, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("claim operation: %v", err)
	}
	if claimed == nil || claimed.ID != operation.ID {
		t.Fatalf("unexpected claimed operation: %+v", claimed)
	}
	blockedClaim, err := repository.ClaimNextOperation(ctx, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("check competing lease: %v", err)
	}
	if blockedClaim != nil {
		t.Fatalf("second worker claimed an active lease: %+v", blockedClaim)
	}

	if err := claimed.Transition(domain.OperationValidating, now.Add(time.Second)); err != nil {
		t.Fatalf("transition to validating: %v", err)
	}
	if err := claimed.Transition(domain.OperationRunning, now.Add(2*time.Second)); err != nil {
		t.Fatalf("transition to running: %v", err)
	}
	completedAt := now.Add(3 * time.Second)
	claimed.Items[0].Status = domain.ItemSucceeded
	claimed.Items[0].StartedAt = claimed.StartedAt
	claimed.Items[0].CompletedAt = &completedAt
	claimed.Items[0].UpdatedAt = completedAt
	if err := claimed.Transition(domain.OperationSucceeded, completedAt); err != nil {
		t.Fatalf("transition to succeeded: %v", err)
	}
	if err := repository.SaveOperation(ctx, claimed); err != nil {
		t.Fatalf("save completed operation: %v", err)
	}
	if err := repository.ApplyOperationItemResult(ctx, postgresTestClassroomID, postgresTestSeatID, domain.OperationRestore, domain.ItemSucceeded); err != nil {
		t.Fatalf("apply operation result: %v", err)
	}

	classroom, err := repository.GetClassroom(ctx, postgresTestClassroomID)
	if err != nil {
		t.Fatalf("get updated classroom: %v", err)
	}
	if len(classroom.Seats) != 1 || classroom.Seats[0].OperationState != domain.ItemSucceeded {
		t.Fatalf("seat state was not persisted: %+v", classroom.Seats)
	}
	if classroom.Seats[0].Desktop == nil || classroom.Seats[0].Desktop.ObservedState != domain.PowerRunning {
		t.Fatalf("desktop state was not updated: %+v", classroom.Seats[0].Desktop)
	}
	if classroom.Seats[0].Desktop.BaselineSnapshot != "classroom-baseline" {
		t.Fatalf("desktop baseline snapshot was not loaded: %+v", classroom.Seats[0].Desktop)
	}

	repository.Close()
	repository = nil
	repository, err = NewPostgresRepository(ctx, scopedURL)
	if err != nil {
		t.Fatalf("reopen PostgreSQL repository: %v", err)
	}
	persisted, err := repository.GetOperation(ctx, operation.ID)
	if err != nil {
		t.Fatalf("load persisted operation: %v", err)
	}
	if persisted.Status != domain.OperationSucceeded || persisted.Counts.Succeeded != 1 || persisted.Items[0].Status != domain.ItemSucceeded || persisted.Items[0].SnapshotName != "classroom-baseline" {
		t.Fatalf("operation did not survive repository restart: %+v", persisted)
	}
}

func TestDatabaseURLWithSearchPath(t *testing.T) {
	t.Parallel()
	result, err := databaseURLWithSearchPath("postgres://user:pass@db.example/classroom?sslmode=require", "pve_test_schema")
	if err != nil {
		t.Fatalf("scope database URL: %v", err)
	}
	parsed, err := url.Parse(result)
	if err != nil {
		t.Fatalf("parse scoped URL: %v", err)
	}
	if parsed.Query().Get("sslmode") != "require" || parsed.Query().Get("search_path") != "pve_test_schema" {
		t.Fatalf("unexpected scoped URL %q", result)
	}
	if _, err := databaseURLWithSearchPath("host=localhost dbname=classroom", "pve_test_schema"); err == nil {
		t.Fatal("keyword DSN must be rejected by this isolated-schema helper")
	}
}

const (
	postgresTestOrganizationID = "10000000-0000-4000-8000-000000000001"
	postgresTestClassroomID    = "10000000-0000-4000-8000-000000000002"
	postgresTestSeatID         = "10000000-0000-4000-8000-000000000003"
	postgresTestDesktopID      = "10000000-0000-4000-8000-000000000004"
	postgresTestClusterID      = "10000000-0000-4000-8000-000000000005"
	postgresTestOperationID    = "10000000-0000-4000-8000-000000000006"
	postgresTestItemID         = "10000000-0000-4000-8000-000000000007"
)

func databaseURLWithSearchPath(databaseURL, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", errors.New("PVE_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func applyTestMigration(t *testing.T, ctx context.Context, databaseURL string) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve integration test location")
	}
	migrationPaths, err := filepath.Glob(filepath.Join(filepath.Dir(filename), "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	if len(migrationPaths) == 0 {
		t.Fatal("no migrations found")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse migration database URL: %v", err)
	}
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open migration connection: %v", err)
	}
	defer pool.Close()
	for _, migrationPath := range migrationPaths {
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("read migration %s: %v", filepath.Base(migrationPath), err)
		}
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %s: %v", filepath.Base(migrationPath), err)
		}
	}
}

func seedPostgresRepository(t *testing.T, ctx context.Context, databaseURL string) {
	t.Helper()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open seed connection: %v", err)
	}
	defer pool.Close()
	now := time.Now().UTC()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO organizations (id, name) VALUES ($1, 'Test School')`, []any{postgresTestOrganizationID}},
		{`INSERT INTO classrooms (id, organization_id, name, site, status, timezone, template_name, template_version, updated_at)
		  VALUES ($1, $2, 'Integration Classroom', 'Test Site', 'READY', 'Asia/Shanghai', 'Windows 11', '1.0.0', $3)`, []any{postgresTestClassroomID, postgresTestOrganizationID, now}},
		{`INSERT INTO seats (id, classroom_id, label, operation_state, updated_at)
		  VALUES ($1, $2, '01', 'SUCCEEDED', $3)`, []any{postgresTestSeatID, postgresTestClassroomID, now}},
		{`INSERT INTO virtual_desktops (id, seat_id, cluster_id, pve_vmid, name, desired_state, observed_state, template_version, baseline_snapshot_name, guest_agent_ready, config_hash, updated_at)
		  VALUES ($1, $2, $3, 101, 'student-test-01', 'STOPPED', 'STOPPED', '1.0.0', 'classroom-baseline', false, 'test-config', $4)`, []any{postgresTestDesktopID, postgresTestSeatID, postgresTestClusterID, now}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed PostgreSQL repository: %v", err)
		}
	}
}

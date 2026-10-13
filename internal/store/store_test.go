package store_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"onsite-ai/internal/store"
	"onsite-ai/internal/store/storetest"
)

var wantTables = []string{
	"admin_users",
	"agents",
	"connectors",
	"enrolment_tokens",
	"gpus",
	"machine_sources",
	"machines",
	"scans",
	"utilisation_samples",
}

func listTables(t *testing.T, s *store.Store) []string {
	t.Helper()
	rows, err := s.Pool().Query(context.Background(),
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name <> 'schema_migrations'
		 ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}
	return got
}

func TestMigrateUpCreatesEveryTable(t *testing.T) {
	s := storetest.New(t)
	if got := listTables(t, s); !slices.Equal(got, wantTables) {
		t.Errorf("tables after migrating up:\n got  %v\n want %v", got, wantTables)
	}
}

func TestMigrateIsRepeatableAndReversible(t *testing.T) {
	dbURL := storetest.NewURL(t)
	if err := store.MigrateUp(dbURL); err != nil {
		t.Fatalf("first up: %v", err)
	}
	if err := store.MigrateUp(dbURL); err != nil {
		t.Fatalf("second up should be a no-op: %v", err)
	}
	if err := store.MigrateDown(dbURL); err != nil {
		t.Fatalf("down: %v", err)
	}

	s, err := store.Open(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got := listTables(t, s); len(got) != 0 {
		t.Errorf("tables left after migrating down: %v", got)
	}
	s.Close()

	if err := store.MigrateUp(dbURL); err != nil {
		t.Fatalf("up after down: %v", err)
	}
}

func TestMachineSourceMustHaveConnectorUnlessAgent(t *testing.T) {
	s := storetest.New(t)
	ctx := context.Background()
	var machineID string
	if err := s.Pool().QueryRow(ctx,
		`INSERT INTO machines (hostname) VALUES ('node-1') RETURNING id`).Scan(&machineID); err != nil {
		t.Fatalf("insert machine: %v", err)
	}

	tests := []struct {
		name    string
		kind    string
		wantErr bool
	}{
		{"agent without connector", "agent", false},
		{"aws without connector", "aws", true},
		{"unknown kind", "vmware", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Pool().Exec(ctx,
				`INSERT INTO machine_sources (machine_id, kind, external_id, data)
				 VALUES ($1, $2, $3, '{}')`, machineID, tt.kind, tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("insert error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedactRemovesPassword(t *testing.T) {
	const dbURL = "postgres://onsite:s3cr3t-pw@db:5432/onsite"
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{"password echoed", "dial postgres://onsite:s3cr3t-pw@db:5432 failed", "dial postgres://onsite:[redacted]@db:5432 failed"},
		{"nothing to hide", "connection refused", "connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := store.Redact(errors.New(tt.msg), dbURL).Error()
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "s3cr3t-pw") {
				t.Errorf("password leaked: %q", got)
			}
		})
	}
}

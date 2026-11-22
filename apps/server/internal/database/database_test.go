package database

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestInstallationMigration(t *testing.T) {
	url := os.Getenv("CRYOL_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("CRYOL_TEST_DATABASE_URL requires a disposable empty database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := Check(ctx, pool); err == nil {
		t.Fatal("an uninitialized database passed readiness")
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var before, after string
	if err := pool.QueryRow(ctx, "SELECT id::text FROM installation_info").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT id::text FROM installation_info").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("rerunning migrations changed installation identity")
	}
}

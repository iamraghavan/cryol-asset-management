package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/database/dbsql"
	"github.com/cryol/cryol-asset-management/apps/server/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid CRYOL_DATABASE_URL")
	}
	cfg.MaxConns = 12
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not initialize the database pool")
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.Files, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return err
	}
	return Check(ctx, pool)
}

func Check(ctx context.Context, pool *pgxpool.Pool) error {
	const target = 5
	var current int64
	err := pool.QueryRow(ctx, "SELECT COALESCE(max(version_id), 0) FROM goose_db_version").Scan(&current)
	if err != nil {
		return err
	}
	if current != target {
		return fmt.Errorf("database schema version %d, expected %d", current, target)
	}
	exists, err := dbsql.New(pool).InstallationExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("database is missing its Cryol installation identity")
	}
	return nil
}

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cryol/cryol-asset-management/apps/server/internal/auth"
	"github.com/cryol/cryol-asset-management/apps/server/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"
)

func runSetup(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	envPath := flags.String("write-env", "", "create a new shell-compatible environment file")
	backupPath := flags.String("restore", "", "restore a trusted Cryol pg_dump archive into an empty database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("run cryol-cli setup [--write-env PATH] [--restore BACKUP] in an interactive terminal")
	}
	if *envPath != "" {
		if _, err := os.Lstat(*envPath); !errors.Is(err, os.ErrNotExist) {
			return errors.New("choose an unused environment file path; setup preserves existing files")
		}
	}
	fmt.Fprintln(os.Stderr, "Cryol Setup")
	fmt.Fprintln(os.Stderr, "1. Connect to PostgreSQL")
	dsn := strings.TrimSpace(os.Getenv("CRYOL_DATABASE_URL"))
	if dsn == "" {
		if *envPath == "" {
			return errors.New("set CRYOL_DATABASE_URL or use --write-env PATH to configure a new connection")
		}
		fmt.Fprint(os.Stderr, "Database connection URL (hidden): ")
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		defer clear(value)
		if err != nil {
			return err
		}
		dsn = strings.TrimSpace(string(value))
	}
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	name, existing, err := inspectSetupDatabase(check, pool)
	cancel()
	if err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	fmt.Fprintf(os.Stderr, "2. Prepare database %q on %q\n", name, pool.Config().ConnConfig.Host)
	if *backupPath != "" {
		if existing {
			return errors.New("restore requires an empty database; the existing installation was left unchanged")
		}
		fmt.Fprint(os.Stderr, "Restore only a trusted Cryol backup. Type RESTORE to load it into this empty database: ")
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "RESTORE" {
			return errors.New("setup cancelled before restore")
		}
		if err := restoreSetupBackup(ctx, pool, dsn, *backupPath); err != nil {
			return err
		}
		check, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, existing, err = inspectSetupDatabase(check, pool)
		cancel()
		if err != nil || !existing {
			return errors.New("restore finished but the Cryol installation could not be verified")
		}
	}
	var current bool
	if existing {
		check, cancel := context.WithTimeout(ctx, 10*time.Second)
		current = database.Check(check, pool) == nil
		cancel()
	}
	if !current {
		prompt := "Type APPLY to install the Cryol schema: "
		if existing {
			prompt = "Back up this Cryol database before continuing. Type APPLY after verifying the backup: "
		}
		fmt.Fprint(os.Stderr, prompt)
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "APPLY" {
			return errors.New("setup cancelled before migration")
		}
		migrate, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err = database.Migrate(migrate, pool)
		cancel()
		if err != nil {
			return fmt.Errorf("database migration failed: %w", err)
		}
	}
	fmt.Fprintln(os.Stderr, "3. Check administrator access")
	var users bool
	check, cancel = context.WithTimeout(ctx, 10*time.Second)
	err = pool.QueryRow(check, "SELECT EXISTS (SELECT 1 FROM app_user)").Scan(&users)
	cancel()
	if err != nil {
		return errors.New("could not check the existing Cryol accounts")
	}
	if !users {
		fmt.Fprint(os.Stderr, "Administrator username [admin]: ")
		name, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = "admin"
		}
		password, err := readAdminPassword()
		if err != nil {
			return err
		}
		defer clear(password)
		account, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = auth.Bootstrap(account, pool, name, password)
		cancel()
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, "Existing accounts retained. Sign in with your current Cryol account.")
	}
	if *envPath != "" {
		address := strings.TrimSpace(os.Getenv("CRYOL_HTTP_ADDR"))
		if address == "" {
			address = "127.0.0.1:8080"
		}
		if err := writeSetupEnv(*envPath, dsn, address); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Connection settings saved to %q. Load this file before starting Cryol.\n", *envPath)
	}
	fmt.Fprintln(os.Stderr, "Database setup complete. Start cryol-server with your Cryol environment.")
	return nil
}

func restoreSetupBackup(ctx context.Context, pool *pgxpool.Pool, dsn, path string) error {
	connection, err := url.Parse(dsn)
	if err != nil || (connection.Scheme != "postgres" && connection.Scheme != "postgresql") ||
		connection.User == nil || connection.Host == "" || connection.Path == "" {
		return errors.New("restore requires a PostgreSQL URL with a host, username and database name")
	}
	cfg := pool.Config().ConnConfig
	if connection.Hostname() != cfg.Host || strings.Contains(cfg.Host, ",") ||
		strings.TrimPrefix(connection.Path, "/") != cfg.Database || connection.User.Username() != cfg.User {
		return errors.New("restore requires a PostgreSQL URL with a single explicit server and database")
	}
	options, err := url.ParseQuery(connection.RawQuery)
	if err != nil {
		return errors.New("invalid PostgreSQL restore connection options")
	}
	for key, values := range options {
		switch key {
		case "sslmode", "sslrootcert", "sslcert", "sslkey":
			if len(values) == 1 {
				continue
			}
		}
		return errors.New("restore supports only explicit PostgreSQL URL credentials and TLS options")
	}
	connection.Host = net.JoinHostPort(cfg.Host, strconv.Itoa(int(cfg.Port)))
	connection.User = url.User(connection.User.Username())
	backup, err := filepath.Abs(path)
	if err != nil {
		return errors.New("invalid backup path")
	}
	info, err := os.Stat(backup)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("the backup must be a readable pg_dump archive file")
	}
	executable, err := exec.LookPath("pg_restore")
	if err != nil {
		return errors.New("install PostgreSQL command-line tools and add pg_restore to PATH")
	}
	list, cancel := context.WithTimeout(ctx, 15*time.Second)
	catalog, err := exec.CommandContext(list, executable, "--list", backup).Output()
	cancel()
	if err != nil || !strings.Contains(string(catalog), " TABLE public installation_info ") ||
		!strings.Contains(string(catalog), " TABLE public goose_db_version ") {
		return errors.New("the archive does not contain the required Cryol installation and migration tables")
	}
	restore := exec.CommandContext(ctx, executable, "--dbname", connection.String(),
		"--no-password", "--single-transaction", "--no-owner", "--no-privileges", backup)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(value), "PG") {
			restore.Env = append(restore.Env, value)
		}
	}
	restore.Env = append(restore.Env, "PGPASSWORD="+cfg.Password)
	if err := restore.Run(); err != nil {
		return fmt.Errorf("pg_restore failed (%w); verify the archive and database permissions", err)
	}
	return nil
}

func inspectSetupDatabase(ctx context.Context, pool *pgxpool.Pool) (string, bool, error) {
	var name, schema string
	var version int
	var identity, history, occupied bool
	err := pool.QueryRow(ctx, `SELECT current_database(), coalesce(current_schema(), ''),
current_setting('server_version_num')::int,
to_regclass('public.installation_info') IS NOT NULL,
to_regclass('public.goose_db_version') IS NOT NULL,
EXISTS (SELECT FROM pg_namespace WHERE nspname <> 'public' AND nspname <> 'information_schema' AND nspname !~ '^pg_')
OR EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','S','f'))`).
		Scan(&name, &schema, &version, &identity, &history, &occupied)
	if err != nil {
		return "", false, errors.New("could not connect to PostgreSQL; check the server, database, credentials and TLS settings")
	}
	if version < 170000 || schema != "public" {
		return "", false, errors.New("setup requires PostgreSQL 17 or later and the public schema as the default")
	}
	if identity && history {
		var valid bool
		if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM public.installation_info WHERE singleton AND id IS NOT NULL)").Scan(&valid); err != nil || !valid {
			return "", false, errors.New("could not verify this database as an existing Cryol installation")
		}
		return name, true, nil
	}
	if occupied {
		return "", false, errors.New("the database contains objects but has no verified Cryol installation; choose a dedicated empty database or a complete Cryol restore")
	}
	return name, false, nil
}

func writeSetupEnv(path, dsn, address string) error {
	if strings.ContainsAny(dsn+address, "\r\n\x00") {
		return errors.New("connection settings must be single-line values")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("could not create the environment file: %w", err)
	}
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	_, err = fmt.Fprintf(file, "CRYOL_DATABASE_URL=%s\nCRYOL_HTTP_ADDR=%s\n", quote(dsn), quote(address))
	if err := errors.Join(err, file.Close()); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("could not save the environment file: %w", err)
	}
	return nil
}

package main

// Databasen er PostgreSQL (opgaven "Upgrade the Database", valgt i GitHub Discussion #78).
// Intet ORM: vi skriver SQL selv med database/sql og driveren pgx.
// Tabellerne laves af migreringerne i migrations/ med goose.

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DATABASE_URL sættes i compose.yml (lokalt og i CI) og compose.prod.yml (serveren).
// Der står ikke et password i koden.
func databaseURL() string {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is not set, e.g. postgres://user:password@localhost:5432/whoknows?sslmode=disable")
	}
	return url
}

// Den gamle SQLite-database på serveren (mountet read-only i compose.prod.yml).
// Importen kører kun, når SQLITE_IMPORT=true.
const sqliteImportPath = "app.db"

// initDB forbinder til PostgreSQL, kører migreringerne og gør gamle brugere klar
func initDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Opstarten har ingen request, så vi bruger en baggrunds-context
	ctx := context.Background()

	if err := waitForDatabase(ctx, db, 30*time.Second); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	if err := migrateUsers(ctx, db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to migrate users: %w", err)
	}

	return db, nil
}

// waitForDatabase venter på, at PostgreSQL er klar. I Docker Compose kan appen starte,
// før databasen tager imod forbindelser.
func waitForDatabase(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := db.PingContext(ctx)
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(time.Second)
	}
}

// runMigrations kører de migreringer i migrations/, der ikke er kørt endnu
func runMigrations(ctx context.Context, db *sql.DB) error {
	migrations, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations)
	if err != nil {
		return err
	}
	results, err := provider.Up(ctx)
	for _, result := range results {
		log.Printf("Migration %s done in %s", result.Source.Path, result.Duration)
	}
	return err
}

//==============================================================================================================================================================================
// IMPORT FRA SQLITE
//
// Første gang appen starter med PostgreSQL på serveren, er databasen tom. Hvis
// SQLITE_IMPORT=true, kopieres brugere og sider fra den gamle app.db.
// Det sker kun, når der ingen sider er i PostgreSQL, så det kan ikke ske to gange.
//==============================================================================================================================================================================

func importFromSQLite(ctx context.Context, db *sql.DB, sqlitePath string) error {
	if sqlitePath == "" {
		return nil
	}
	sqlitePath = filepath.Clean(sqlitePath)
	if _, err := os.Stat(sqlitePath); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	var pages int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pages").Scan(&pages); err != nil {
		return err
	}
	if pages > 0 {
		return nil
	}

	old, err := sql.Open("sqlite", "file:"+sqlitePath+"?mode=ro")
	if err != nil {
		return err
	}
	defer func() { _ = old.Close() }()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	users, err := copyUsers(ctx, old, tx)
	if err != nil {
		return fmt.Errorf("users: %w", err)
	}
	copied, err := copyPages(ctx, old, tx)
	if err != nil {
		return fmt.Errorf("pages: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("Imported %d users and %d pages from SQLite", users, copied)
	return nil
}

func copyUsers(ctx context.Context, old *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := old.QueryContext(ctx, "SELECT username, email, password, must_change_password FROM users")
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		var username, email, password string
		var mustChange int
		if err := rows.Scan(&username, &email, &password, &mustChange); err != nil {
			return count, err
		}
		// Standard-admin fra migreringen (password admin123) overskrives af admin fra SQLite,
		// så serverens rigtige admin-password bliver bevaret
		result, err := tx.ExecContext(ctx,
			`INSERT INTO users (username, email, password, must_change_password) VALUES ($1, $2, $3, $4)
			ON CONFLICT (username) DO UPDATE SET
				email = EXCLUDED.email,
				password = EXCLUDED.password,
				must_change_password = EXCLUDED.must_change_password`,
			username, email, password, mustChange != 0)
		if err != nil {
			return count, err
		}
		if n, _ := result.RowsAffected(); n > 0 {
			count++
		}
	}
	return count, rows.Err()
}

func copyPages(ctx context.Context, old *sql.DB, tx *sql.Tx) (int, error) {
	rows, err := old.QueryContext(ctx, "SELECT title, url, language, COALESCE(CAST(last_updated AS TEXT), ''), content FROM pages")
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		var title, url, language, lastUpdated, content string
		if err := rows.Scan(&title, &url, &language, &lastUpdated, &content); err != nil {
			return count, err
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO pages (title, url, language, last_updated, content) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING",
			title, url, language, parseTimestamp(lastUpdated), content)
		if err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}

// parseTimestamp læser de tidsformater, SQLite-databasen har gemt. Ukendte formater bliver NULL.
func parseTimestamp(value string) any {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05", time.DateOnly} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return nil
}

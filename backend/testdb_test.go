package main

// Testene kører mod en rigtig PostgreSQL. TEST_DATABASE_URL peger på den (i CI en
// PostgreSQL-service, lokalt fx compose.yml). Hver test får sit eget schema, så testene
// ikke ser hinandens data, og schemaet slettes igen bagefter.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
)

func testDatabaseURL(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		// I CI skal testene køre, så der må de ikke springes over
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI")
		}
		t.Skip("TEST_DATABASE_URL is not set (start PostgreSQL with docker compose up postgres)")
	}
	return dsn
}

// openTestDB laver et nyt schema, kører migreringerne i det og sætter den globale db
func openTestDB(t testing.TB) {
	t.Helper()
	base := testDatabaseURL(t)

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	random := make([]byte, 6)
	_, _ = rand.Read(random)
	schema := "test_" + hex.EncodeToString(random)
	if _, err := admin.ExecContext(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	db, err = initDB(u.String())
	if err != nil {
		t.Fatalf("initDB: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close()
	})
}

package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestPostgresNamesDatabase(t *testing.T) {
	e := postgres{}
	for s, want := range map[string]bool{
		"host=localhost dbname=chinook":           true,
		"host=localhost user=me":                  false,
		"postgres://me:pw@localhost:5432/chinook": true,
		"postgres://me:pw@localhost:5432":         false,
		"postgresql://localhost/":                 false,
	} {
		if got := e.namesDatabase(s); got != want {
			t.Errorf("%s: got %v", s, got)
		}
	}
}

func TestPostgresLiteral(t *testing.T) {
	e := postgres{}
	cases := []struct {
		v    any
		base string
		want string
	}{
		{nil, "int4", "NULL"},
		{int64(7), "int4", "7"},
		{true, "bool", "true"},
		{"it's", "text", "'it''s'"},
		{"12.30", "numeric", "'12.30'"},
		{[]byte{0xCA, 0xFE}, "bytea", `'\xcafe'`},
		{[]byte(`{"a":1}`), "jsonb", `'{"a":1}'`},
	}
	for _, c := range cases {
		if got := e.literal(c.v, c.base); got != c.want {
			t.Errorf("%v (%s): got %s, want %s", c.v, c.base, got, c.want)
		}
	}
}

// TestPostgresPagila reads the Pagila sample database. It runs only when GLDB_TEST_PG
// holds a connection string to it, e.g. GLDB_TEST_PG="host=localhost dbname=pagila".
func TestPostgresPagila(t *testing.T) {
	s := os.Getenv("GLDB_TEST_PG")
	if s == "" {
		t.Skip("GLDB_TEST_PG not set")
	}
	e, ctx := postgres{}, context.Background()
	db, err := e.open(s, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if info, err := e.serverInfo(ctx, db); err != nil || !strings.HasPrefix(info, "PostgreSQL ") {
		t.Fatalf("server: %q %v", info, err)
	}
	list, err := e.tables(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]table{}
	for _, tb := range list {
		found[tb.full()] = tb
	}
	payment, film := found["public.payment"], found["public.film"]
	if payment.Rows == 0 || film.Rows == 0 {
		t.Fatalf("rows: payment %d, film %d", payment.Rows, film.Rows)
	}

	info, err := e.describe(ctx, db, film)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.primaryKey(); len(got) != 1 || got[0] != "film_id" {
		t.Errorf("primary key %v", got)
	}
	if len(info.FKs) != 2 || info.FKs[0].RefTable != "public.language" {
		t.Errorf("foreign keys %+v", info.FKs)
	}
	page, err := readPage(ctx, e, db, info, 1)
	if err != nil || len(page) != pageSize {
		t.Fatalf("page: %d rows, %v", len(page), err)
	}
	var out bytes.Buffer
	n, err := exportRows(ctx, e, db, info, exportDropCreate, &out, func(int64) {})
	if err != nil || n == 0 {
		t.Fatalf("export: %d rows, %v", n, err)
	}
	if !strings.HasPrefix(out.String(), `DROP TABLE IF EXISTS "public"."film";`) {
		t.Errorf("export starts with %.80q", out.String())
	}

	// A partitioned table: its partitions go in the script.
	info, err = e.describe(ctx, db, payment)
	if err != nil {
		t.Fatal(err)
	}
	if info.PartitionBy == "" || len(info.Partitions) == 0 || !strings.Contains(e.createScript(info), " PARTITION OF ") {
		t.Errorf("partitions: %q, %d", info.PartitionBy, len(info.Partitions))
	}
}

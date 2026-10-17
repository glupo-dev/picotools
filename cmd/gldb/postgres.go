package main

// PostgreSQL: driver, catalog queries and SQL.

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type postgres struct{}

func (postgres) name() string { return "PostgreSQL" }

// The connection string is a postgres:// address or keywords as in
// "host=localhost port=5432 dbname=shop user=me password=secret".
func (postgres) check(s string) error {
	_, err := pgx.ParseConfig(s)
	return err
}

func (e postgres) where(s string) string {
	cfg, err := pgx.ParseConfig(s)
	if err != nil {
		return "connection string not valid"
	}
	db := cfg.Database
	if !e.namesDatabase(s) {
		db = "(choose database)"
	}
	return fmt.Sprintf("%s:%d · %s", cfg.Host, cfg.Port, db)
}

// namesDatabase looks at the string itself: when it names no database the driver
// would take the user name, which is not what the user asked.
func (postgres) namesDatabase(s string) bool {
	if strings.HasPrefix(s, "postgres://") || strings.HasPrefix(s, "postgresql://") {
		u, err := url.Parse(s)
		return err == nil && strings.Trim(u.Path, "/") != ""
	}
	for _, f := range strings.Fields(s) {
		if k, v, ok := strings.Cut(f, "="); ok && strings.TrimSpace(k) == "dbname" && strings.Trim(v, "'") != "" {
			return true
		}
	}
	return false
}

// open uses the database asked, or the one in the string; with neither it opens
// "postgres", which every server has, to list the others.
func (e postgres) open(s, database string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(s)
	if err != nil {
		return nil, err
	}
	switch {
	case database != "":
		cfg.Database = database
	case !e.namesDatabase(s):
		cfg.Database = "postgres"
	}
	return stdlib.OpenDB(*cfg), nil
}

func (postgres) serverInfo(ctx context.Context, db *sql.DB) (string, error) {
	var version string
	err := db.QueryRowContext(ctx, "SHOW server_version").Scan(&version)
	return "PostgreSQL " + version, err
}

func (postgres) currentDatabase(ctx context.Context, db *sql.DB) (string, error) {
	var name string
	err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&name)
	return name, err
}

// databases lists the databases of the server that the user can open.
func (postgres) databases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT datname FROM pg_database
		WHERE datallowconn AND NOT datistemplate AND has_database_privilege(datname, 'CONNECT')
		ORDER BY datname`)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// quote puts a name in double quotes, doubling the " inside.
func (postgres) quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

// pgRowCount is read from the statistics: immediate also on huge tables, and close
// to the real number. A partitioned table has no rows of its own: its rows are the
// sum of its partitions.
const pgRowCount = `CASE WHEN c.relkind = 'p' THEN
		COALESCE((SELECT SUM(COALESCE(s.n_live_tup, GREATEST(p.reltuples, 0)))
			FROM pg_partition_tree(c.oid) pt
			JOIN pg_class p ON p.oid = pt.relid
			LEFT JOIN pg_stat_all_tables s ON s.relid = p.oid
			WHERE pt.isleaf), 0)
	ELSE COALESCE((SELECT s.n_live_tup FROM pg_stat_all_tables s WHERE s.relid = c.oid), GREATEST(c.reltuples, 0))
	END::bigint`

// tables lists the user tables of the database, by schema and name (partitions are
// left out: their table is listed).
func (postgres) tables(ctx context.Context, db *sql.DB) ([]table, error) {
	rows, err := db.QueryContext(ctx, `SELECT n.nspname, c.relname, `+pgRowCount+`
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p') AND NOT c.relispartition
			AND n.nspname <> 'information_schema' AND n.nspname NOT LIKE 'pg\_%'
		ORDER BY n.nspname, c.relname`)
	if err != nil {
		return nil, err
	}
	return scanTables(rows)
}

// sep joins the names of a list read in one column.
const sep = "\x1f"

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}

// describe reads columns, indexes, foreign keys and checks of a table.
func (e postgres) describe(ctx context.Context, db *sql.DB, t table) (*tableInfo, error) {
	info := &tableInfo{table: t}
	name := quoted(e, t)

	err := db.QueryRowContext(ctx, `SELECT `+pgRowCount+`, COALESCE(pg_get_partkeydef(c.oid), '')
		FROM pg_class c WHERE c.oid = $1::regclass`, name).Scan(&info.Rows, &info.PartitionBy)
	if err != nil {
		return nil, err
	}
	if info.PartitionBy != "" {
		// The partitions, each with its range or list of values, so the script can make them too.
		rows, err := db.QueryContext(ctx, `SELECT n.nspname, p.relname, pg_get_expr(p.relpartbound, p.oid)
			FROM pg_inherits i
			JOIN pg_class p ON p.oid = i.inhrelid
			JOIN pg_namespace n ON n.oid = p.relnamespace
			WHERE i.inhparent = $1::regclass
			ORDER BY p.relname`, name)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var schema, part, bound string
			if err := rows.Scan(&schema, &part, &bound); err != nil {
				rows.Close()
				return nil, err
			}
			info.Partitions = append(info.Partitions, fmt.Sprintf("CREATE TABLE %s.%s PARTITION OF %s %s;",
				e.quote(schema), e.quote(part), name, bound))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	rows, err := db.QueryContext(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), t.typname,
			NOT a.attnotnull, a.attidentity::text, a.attgenerated::text,
			COALESCE(pg_get_expr(d.adbin, d.adrelid), ''),
			COALESCE(s.seqstart::text, ''), COALESCE(s.seqincrement::text, '')
		FROM pg_attribute a
		JOIN pg_type t ON t.oid = a.atttypid
		LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		LEFT JOIN pg_sequence s ON s.seqrelid = pg_get_serial_sequence($1, a.attname)::regclass
		WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c column
		var identity, generated, def string
		if err := rows.Scan(&c.Name, &c.Type, &c.Base, &c.Nullable, &identity, &generated, &def, &c.Seed, &c.Increment); err != nil {
			rows.Close()
			return nil, err
		}
		switch {
		case generated == "s" || generated == "v": // stored, or virtual (PostgreSQL 18)
			c.Computed, c.Persisted, c.ReadOnly = def, generated == "s", true
		case identity == "a" || identity == "d":
			c.Identity, c.Always = true, identity == "a"
		case strings.HasPrefix(def, "nextval("):
			c.Identity, c.Serial = true, true
		default:
			c.Default = def
		}
		info.Columns = append(info.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT ic.relname, am.amname, i.indisprimary, i.indisunique, con.oid IS NOT NULL,
			CASE WHEN con.oid IS NULL THEN pg_get_indexdef(i.indexrelid) ELSE pg_get_constraintdef(con.oid) END,
			i.indnkeyatts, i.indoption::text,
			array_to_string(ARRAY(SELECT pg_get_indexdef(i.indexrelid, k, true) FROM generate_series(1, i.indnatts) k), E'\x1f'),
			COALESCE(pg_get_expr(i.indpred, i.indrelid), '')
		FROM pg_index i
		JOIN pg_class ic ON ic.oid = i.indexrelid
		JOIN pg_am am ON am.oid = ic.relam
		LEFT JOIN pg_constraint con ON con.conindid = i.indexrelid AND con.conrelid = i.indrelid AND con.contype IN ('p', 'u', 'x')
		WHERE i.indrelid = $1::regclass
		ORDER BY i.indisprimary DESC, con.oid IS NULL, ic.relname`, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ix index
		var nkeys int
		var options, cols string
		if err := rows.Scan(&ix.Name, &ix.Kind, &ix.Primary, &ix.Unique, &ix.Constraint, &ix.Def, &nkeys, &options, &cols, &ix.Filter); err != nil {
			rows.Close()
			return nil, err
		}
		all := split(cols)
		opts := strings.Fields(options)
		for k, col := range all {
			if k >= nkeys {
				ix.Include = append(ix.Include, col)
				continue
			}
			ix.Keys = append(ix.Keys, col)
			o, _ := strconv.Atoi(opts[min(k, len(opts)-1)])
			ix.Desc = append(ix.Desc, o&1 == 1)
		}
		info.Indexes = append(info.Indexes, ix)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT con.conname, pg_get_constraintdef(con.oid),
			array_to_string(ARRAY(SELECT a.attname FROM unnest(con.conkey) WITH ORDINALITY k(n, o)
				JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = k.n ORDER BY k.o), E'\x1f'),
			rn.nspname || '.' || rc.relname,
			array_to_string(ARRAY(SELECT a.attname FROM unnest(con.confkey) WITH ORDINALITY k(n, o)
				JOIN pg_attribute a ON a.attrelid = con.confrelid AND a.attnum = k.n ORDER BY k.o), E'\x1f')
		FROM pg_constraint con
		JOIN pg_class rc ON rc.oid = con.confrelid
		JOIN pg_namespace rn ON rn.oid = rc.relnamespace
		WHERE con.conrelid = $1::regclass AND con.contype = 'f'
		ORDER BY con.conname`, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var fk foreignKey
		var cols, refCols string
		if err := rows.Scan(&fk.Name, &fk.Def, &cols, &fk.RefTable, &refCols); err != nil {
			rows.Close()
			return nil, err
		}
		fk.Cols, fk.RefCols = split(cols), split(refCols)
		info.FKs = append(info.FKs, fk)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT conname, pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = $1::regclass AND contype = 'c' ORDER BY conname`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c check
		if err := rows.Scan(&c.Name, &c.Def); err != nil {
			return nil, err
		}
		info.Checks = append(info.Checks, c)
	}
	return info, rows.Err()
}

// selectAll orders by primary key, otherwise as PostgreSQL returns the rows.
func (e postgres) selectAll(t *tableInfo, page int) string {
	q := "SELECT " + columnList(e, t) + " FROM " + quoted(e, t.table)
	if order := orderBy(e, t); order != "" {
		q += " ORDER BY " + order
	}
	if page >= 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize, page*pageSize)
	}
	return q
}

// ---- CREATE TABLE ----

// createScript writes the CREATE TABLE with its constraints, as PostgreSQL itself
// describes them, then the other indexes.
func (e postgres) createScript(t *tableInfo) string {
	var b strings.Builder
	b.WriteString("CREATE TABLE " + quoted(e, t.table) + " (\n")
	var parts []string
	for _, c := range t.Columns {
		parts = append(parts, "    "+e.columnDef(c))
	}
	for _, ix := range t.Indexes {
		if ix.Constraint {
			parts = append(parts, "    CONSTRAINT "+e.quote(ix.Name)+" "+ix.Def)
		}
	}
	for _, fk := range t.FKs {
		parts = append(parts, "    CONSTRAINT "+e.quote(fk.Name)+" "+fk.Def)
	}
	for _, c := range t.Checks {
		parts = append(parts, "    CONSTRAINT "+e.quote(c.Name)+" "+c.Def)
	}
	b.WriteString(strings.Join(parts, ",\n"))
	b.WriteString("\n)")
	if t.PartitionBy != "" {
		b.WriteString(" PARTITION BY " + t.PartitionBy)
	}
	b.WriteString(";\n")
	for _, p := range t.Partitions {
		b.WriteString(p + "\n")
	}
	for _, ix := range t.Indexes {
		if !ix.Constraint {
			b.WriteString(ix.Def + ";\n")
		}
	}
	return b.String()
}

// serials are the types written for a column whose default comes from its own sequence:
// the script then makes the sequence too.
var serials = map[string]string{"integer": "serial", "bigint": "bigserial", "smallint": "smallserial"}

func (e postgres) columnDef(c column) string {
	s := e.quote(c.Name) + " " + c.Type
	switch {
	case c.Serial && serials[c.Type] != "":
		s = e.quote(c.Name) + " " + serials[c.Type]
	case c.Identity && c.Always:
		s += " GENERATED ALWAYS AS IDENTITY"
	case c.Identity && !c.Serial:
		s += " GENERATED BY DEFAULT AS IDENTITY"
	case c.Computed != "" && c.Persisted:
		s += " GENERATED ALWAYS AS (" + c.Computed + ") STORED"
	case c.Computed != "":
		s += " GENERATED ALWAYS AS (" + c.Computed + ") VIRTUAL"
	}
	if !c.Nullable {
		s += " NOT NULL"
	}
	if c.Default != "" {
		s += " DEFAULT " + c.Default
	}
	return s
}

// ---- Values ----

// text: booleans as true/false, dates in ISO form, bytea as \x…, JSON as text.
func (postgres) text(v any, base string) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", true
	case bool:
		return strconv.FormatBool(x), false
	case int64:
		return strconv.FormatInt(x, 10), false
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), false
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32), false
	case string:
		return x, false
	case time.Time:
		switch base {
		case "date":
			return x.Format("2006-01-02"), false
		case "timestamptz":
			return x.Format("2006-01-02 15:04:05.999999-07:00"), false
		}
		return x.Format("2006-01-02 15:04:05.999999"), false
	case []byte:
		if base == "bytea" {
			return `\x` + hex.EncodeToString(x), false
		}
		return string(x), false // json, jsonb and other text sent as bytes
	}
	return fmt.Sprint(v), false
}

func (e postgres) literal(v any, base string) string {
	s, null := e.text(v, base)
	if null {
		return "NULL"
	}
	switch x := v.(type) {
	case bool, int64:
		return s
	case float64:
		if !isFinite(x) {
			return sqlString(s)
		}
		return s
	case float32:
		if !isFinite(float64(x)) {
			return sqlString(s)
		}
		return s
	}
	return sqlString(s)
}

func isFinite(f float64) bool { return f == f && f-f == 0 }

// ---- INSERT scripts ----

func (e postgres) dropScript(t *tableInfo) string {
	return "DROP TABLE IF EXISTS " + quoted(e, t.table) + ";"
}

// insertPrefix: identity columns GENERATED ALWAYS take the values of the script only
// with OVERRIDING SYSTEM VALUE.
func (e postgres) insertPrefix(t *tableInfo, cols []string) string {
	s := "INSERT INTO " + quoted(e, t.table) + " (" + strings.Join(cols, ", ") + ")"
	for _, c := range t.Columns {
		if c.Always {
			s += " OVERRIDING SYSTEM VALUE"
			break
		}
	}
	return s + " VALUES ("
}

func (postgres) beforeInserts(*tableInfo) string { return "" }

// afterInserts moves the sequence of each identity or serial column past the values
// inserted, so the next rows added by hand do not collide.
func (e postgres) afterInserts(t *tableInfo) string {
	name := quoted(e, t.table)
	var b strings.Builder
	for _, c := range t.Columns {
		if c.Identity {
			fmt.Fprintf(&b, "SELECT setval(pg_get_serial_sequence(%s, %s), COALESCE(MAX(%s), 0) + 1, false) FROM %s;\n",
				sqlString(name), sqlString(c.Name), e.quote(c.Name), name)
		}
	}
	return b.String()
}

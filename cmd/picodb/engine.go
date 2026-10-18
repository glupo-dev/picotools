package main

// What every database does in the same way, and what each one has to say for itself.
// Pico DB only reads: SELECTs on the data and on the system catalog. The scripts it
// makes are text for the user and are never run.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/csv"
	"io"
	"strings"
)

// engine is one kind of database (SQL Server, PostgreSQL): how to connect, how to read
// its catalog and how to write its SQL.
type engine interface {
	name() string // e.g. "SQL Server"

	// check says what is wrong with a connection string, if anything.
	check(connString string) error
	// where is the server and database of a connection string, for the lists: never the password.
	where(connString string) string
	// namesDatabase says whether the connection string says which database to open.
	namesDatabase(connString string) bool
	// open connects with the connection string; database, when not empty, replaces the one in it.
	open(connString, database string) (*sql.DB, error)

	serverInfo(ctx context.Context, db *sql.DB) (string, error)
	currentDatabase(ctx context.Context, db *sql.DB) (string, error)
	databases(ctx context.Context, db *sql.DB) ([]string, error)
	tables(ctx context.Context, db *sql.DB) ([]table, error)
	describe(ctx context.Context, db *sql.DB, t table) (*tableInfo, error)

	quote(name string) string
	createScript(t *tableInfo) string
	// selectAll is the query of all rows in a stable order; with page >= 0 only that page.
	selectAll(t *tableInfo, page int) string
	// text writes a value as it is shown and exported to CSV; null says it is NULL.
	text(v any, base string) (s string, null bool)
	// literal writes a value as SQL, for the INSERT scripts.
	literal(v any, base string) string

	// Pieces of the INSERT scripts.
	dropScript(t *tableInfo) string
	insertPrefix(t *tableInfo, cols []string) string // up to "VALUES ("
	beforeInserts(t *tableInfo) string
	afterInserts(t *tableInfo) string
}

// Kinds of database, as saved in the connections.
const (
	kindSQLServer = "sqlserver"
	kindPostgres  = "postgres"
)

var kinds = []string{kindSQLServer, kindPostgres}

func engineOf(kind string) engine {
	if kind == kindPostgres {
		return postgres{}
	}
	return sqlServer{} // also connections saved before there was a choice
}

// table is a user table with its number of rows.
type table struct {
	Schema, Name string
	Rows         int64
}

func (t table) full() string { return t.Schema + "." + t.Name }

// quoted is the table name as the database wants it, e.g. [dbo].[Orders] or "public"."album".
func quoted(e engine, t table) string { return e.quote(t.Schema) + "." + e.quote(t.Name) }

// column is a column of a table, as the catalog describes it.
type column struct {
	Name      string
	Type      string // as declared, e.g. nvarchar(50)
	Base      string // the system type, e.g. nvarchar (differs from Type for alias types)
	Nullable  bool
	Identity  bool // filled by the database: identity, or serial in PostgreSQL
	Always    bool // PostgreSQL GENERATED ALWAYS AS IDENTITY: an INSERT must override it
	Seed      string
	Increment string
	Serial    bool   // PostgreSQL: a default taken from a sequence
	Computed  string // the expression of a computed column
	Persisted bool
	Default   string // the expression of the default
	DefName   string // the name of the default constraint, empty when named by the system
	ReadOnly  bool   // cannot be written by an INSERT (computed, rowversion)
}

// index is an index or a primary key / unique constraint.
type index struct {
	Name       string
	Kind       string // CLUSTERED, NONCLUSTERED, btree…
	Primary    bool
	Unique     bool
	Constraint bool // unique constraint (the primary key is one too)
	Keys       []string
	Desc       []bool // for each key, whether it is descending
	Include    []string
	Filter     string
	Def        string // the whole definition, when the database gives it (PostgreSQL)
}

type foreignKey struct {
	Name     string // empty when named by the system
	Cols     []string
	RefTable string
	RefCols  []string
	OnDelete string
	OnUpdate string
	Def      string // the whole definition, when the database gives it (PostgreSQL)
}

type check struct {
	Name string // empty when named by the system
	Def  string
}

// tableInfo is everything the table page and the scripts need.
type tableInfo struct {
	table
	Columns []column
	Indexes []index
	FKs     []foreignKey
	Checks  []check

	PartitionBy string   // PostgreSQL: how a partitioned table splits its rows, e.g. RANGE (payment_date)
	Partitions  []string // PostgreSQL: the CREATE TABLE of each partition
}

// primaryKey gives the columns of the primary key, if any.
func (t *tableInfo) primaryKey() []string {
	for _, ix := range t.Indexes {
		if ix.Primary {
			return append([]string(nil), ix.Keys...)
		}
	}
	return nil
}

// columnList is the columns of the table, quoted and separated by commas.
func columnList(e engine, t *tableInfo) string {
	cols := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		cols[i] = e.quote(c.Name)
	}
	return strings.Join(cols, ", ")
}

// orderBy is the primary key quoted, or "" when the table has none.
func orderBy(e engine, t *tableInfo) string {
	pk := t.primaryKey()
	for i, k := range pk {
		pk[i] = e.quote(k)
	}
	return strings.Join(pk, ", ")
}

// pageSize is how many rows a page of "Select all" shows.
const pageSize = 50

// readPage reads one page of rows; each value is as the driver gives it.
func readPage(ctx context.Context, e engine, db *sql.DB, t *tableInfo, page int) ([][]any, error) {
	rows, err := db.QueryContext(ctx, e.selectAll(t, page))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]any
	for rows.Next() {
		vals, err := scanRow(rows, len(t.Columns))
		if err != nil {
			return nil, err
		}
		out = append(out, vals)
	}
	return out, rows.Err()
}

func scanRow(rows *sql.Rows, n int) ([]any, error) {
	vals := make([]any, n)
	ptrs := make([]any, n)
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	return vals, nil
}

// ---- Exports ----

// export kinds
const (
	exportCSV        = iota // all rows as CSV
	exportInsert            // INSERT of all rows
	exportDropCreate        // DROP + CREATE TABLE + INSERT of all rows
)

// exportRows writes all rows of the table to w in the given kind, calling progress now
// and then with the rows written so far. It stops when ctx is cancelled.
func exportRows(ctx context.Context, e engine, db *sql.DB, t *tableInfo, kind int, w io.Writer, progress func(int64)) (int64, error) {
	bw := bufio.NewWriterSize(w, 1<<16)
	var cw *csv.Writer
	var insert string
	var keep []int // columns written by the INSERT

	if kind == exportCSV {
		cw = csv.NewWriter(bw)
		names := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			names[i] = c.Name
		}
		if err := cw.Write(names); err != nil {
			return 0, err
		}
	} else {
		var names []string
		for i, c := range t.Columns {
			if !c.ReadOnly {
				keep = append(keep, i)
				names = append(names, e.quote(c.Name))
			}
		}
		insert = e.insertPrefix(t, names)
		if kind == exportDropCreate {
			bw.WriteString(e.dropScript(t) + "\n\n")
			bw.WriteString(e.createScript(t))
			bw.WriteString("\n")
		}
		bw.WriteString(e.beforeInserts(t))
	}

	rows, err := db.QueryContext(ctx, e.selectAll(t, -1))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var n int64
	record := make([]string, len(t.Columns))
	values := make([]string, len(keep))
	for rows.Next() {
		vals, err := scanRow(rows, len(t.Columns))
		if err != nil {
			return n, err
		}
		if cw != nil {
			for i, v := range vals {
				record[i], _ = e.text(v, t.Columns[i].Base)
			}
			if err := cw.Write(record); err != nil {
				return n, err
			}
		} else {
			for j, i := range keep {
				values[j] = e.literal(vals[i], t.Columns[i].Base)
			}
			bw.WriteString(insert)
			bw.WriteString(strings.Join(values, ", "))
			if _, err := bw.WriteString(");\n"); err != nil {
				return n, err
			}
		}
		n++
		if n%1000 == 0 {
			progress(n)
		}
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	if cw != nil {
		cw.Flush()
		if err := cw.Error(); err != nil {
			return n, err
		}
	} else {
		bw.WriteString(e.afterInserts(t))
	}
	return n, bw.Flush()
}

// quoteList quotes each name and joins them with commas.
func quoteList(e engine, names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = e.quote(n)
	}
	return strings.Join(q, ", ")
}

// sqlString writes s between single quotes, doubling the quotes inside.
func sqlString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func scanStrings(rows *sql.Rows) ([]string, error) {
	defer rows.Close()
	var list []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}

func scanTables(rows *sql.Rows) ([]table, error) {
	defer rows.Close()
	var list []table
	for rows.Next() {
		var t table
		if err := rows.Scan(&t.Schema, &t.Name, &t.Rows); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

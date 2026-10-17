package main

// SQL Server: driver, catalog queries and T-SQL.

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	mssql "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"
)

type sqlServer struct{}

func (sqlServer) name() string { return "SQL Server" }

// The connection string is as SQL Server tools write it (Server=…;Database=…;User Id=…;
// Password=…) or a sqlserver:// address.
func (sqlServer) check(s string) error {
	_, err := msdsn.Parse(s)
	return err
}

func (sqlServer) where(s string) string {
	cfg, err := msdsn.Parse(s)
	if err != nil {
		return "connection string not valid"
	}
	server := cfg.Host
	if cfg.Instance != "" {
		server += `\` + cfg.Instance
	}
	if cfg.Port != 0 {
		server += fmt.Sprintf(":%d", cfg.Port)
	}
	db := cfg.Database
	if db == "" {
		db = "(choose database)"
	}
	return server + " · " + db
}

func (sqlServer) namesDatabase(s string) bool {
	cfg, err := msdsn.Parse(s)
	return err == nil && cfg.Database != ""
}

func (sqlServer) open(s, database string) (*sql.DB, error) {
	cfg, err := msdsn.Parse(s)
	if err != nil {
		return nil, err
	}
	if database != "" {
		cfg.Database = database
	}
	return sql.OpenDB(mssql.NewConnectorConfig(cfg)), nil
}

// serverInfo says which SQL Server answered, e.g. "SQL Server 2022 (16.0.4135.4)".
func (sqlServer) serverInfo(ctx context.Context, db *sql.DB) (string, error) {
	var version string
	var edition int
	err := db.QueryRowContext(ctx, `SELECT CAST(SERVERPROPERTY('ProductVersion') AS nvarchar(128)),
		CAST(SERVERPROPERTY('EngineEdition') AS int)`).Scan(&version, &edition)
	if err != nil {
		return "", err
	}
	if edition == 5 {
		return "Azure SQL Database (" + version + ")", nil
	}
	years := map[string]string{"11": "2012", "12": "2014", "13": "2016", "14": "2017", "15": "2019", "16": "2022", "17": "2025"}
	major, _, _ := strings.Cut(version, ".")
	if y, ok := years[major]; ok {
		return "SQL Server " + y + " (" + version + ")", nil
	}
	return "SQL Server " + version, nil
}

func (sqlServer) currentDatabase(ctx context.Context, db *sql.DB) (string, error) {
	var name string
	err := db.QueryRowContext(ctx, "SELECT DB_NAME()").Scan(&name)
	return name, err
}

// databases lists the databases of the server that are online and that the user can open.
func (sqlServer) databases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sys.databases
		WHERE state = 0 AND HAS_DBACCESS(name) = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// quote puts a name in square brackets, doubling the ] inside.
func (sqlServer) quote(name string) string { return "[" + strings.ReplaceAll(name, "]", "]]") + "]" }

// msRowCount is read from the system statistics: immediate also on huge tables.
const msRowCount = `COALESCE((SELECT SUM(p.rows) FROM sys.partitions p
	WHERE p.object_id = t.object_id AND p.index_id IN (0, 1)), 0)`

// tables lists the user tables of the database, by schema and name.
func (sqlServer) tables(ctx context.Context, db *sql.DB) ([]table, error) {
	rows, err := db.QueryContext(ctx, `SELECT s.name, t.name, `+msRowCount+`
		FROM sys.tables t JOIN sys.schemas s ON s.schema_id = t.schema_id
		WHERE t.is_ms_shipped = 0
		ORDER BY s.name, t.name`)
	if err != nil {
		return nil, err
	}
	return scanTables(rows)
}

// describe reads columns, indexes, foreign keys and checks of a table.
func (e sqlServer) describe(ctx context.Context, db *sql.DB, t table) (*tableInfo, error) {
	info := &tableInfo{table: t}
	name := quoted(e, t)

	err := db.QueryRowContext(ctx, `SELECT `+msRowCount+` FROM sys.tables t WHERE t.object_id = OBJECT_ID(@p1)`, name).Scan(&info.Rows)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `SELECT c.name, ty.name, TYPE_NAME(c.system_type_id), c.max_length, c.precision, c.scale,
			c.is_nullable, c.is_identity,
			COALESCE(CAST(ic.seed_value AS nvarchar(40)), ''), COALESCE(CAST(ic.increment_value AS nvarchar(40)), ''),
			COALESCE(cc.definition, ''), COALESCE(cc.is_persisted, 0),
			COALESCE(dc.definition, ''), CASE WHEN dc.is_system_named = 0 THEN dc.name ELSE '' END
		FROM sys.columns c
		JOIN sys.types ty ON ty.user_type_id = c.user_type_id
		LEFT JOIN sys.identity_columns ic ON ic.object_id = c.object_id AND ic.column_id = c.column_id
		LEFT JOIN sys.computed_columns cc ON cc.object_id = c.object_id AND cc.column_id = c.column_id
		LEFT JOIN sys.default_constraints dc ON dc.object_id = c.default_object_id
		WHERE c.object_id = OBJECT_ID(@p1)
		ORDER BY c.column_id`, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c column
		var maxLen, prec, scale int
		var defName sql.NullString
		if err := rows.Scan(&c.Name, &c.Type, &c.Base, &maxLen, &prec, &scale, &c.Nullable, &c.Identity,
			&c.Seed, &c.Increment, &c.Computed, &c.Persisted, &c.Default, &defName); err != nil {
			rows.Close()
			return nil, err
		}
		c.DefName = defName.String
		if c.Type == c.Base {
			c.Type = msTypeText(c.Base, maxLen, prec, scale)
		}
		// Computed and rowversion columns are filled by SQL Server.
		c.ReadOnly = c.Computed != "" || c.Base == "timestamp"
		info.Columns = append(info.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT i.name, i.type_desc, i.is_primary_key, i.is_unique, i.is_unique_constraint,
			ic.is_descending_key, ic.is_included_column, c.name, COALESCE(i.filter_definition, '')
		FROM sys.indexes i
		JOIN sys.index_columns ic ON ic.object_id = i.object_id AND ic.index_id = i.index_id
		JOIN sys.columns c ON c.object_id = ic.object_id AND c.column_id = ic.column_id
		WHERE i.object_id = OBJECT_ID(@p1) AND i.index_id > 0 AND i.is_hypothetical = 0
		ORDER BY i.is_primary_key DESC, i.is_unique_constraint DESC, i.name, ic.is_included_column, ic.key_ordinal, ic.index_column_id`, name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ix index
		var desc, included bool
		var col string
		if err := rows.Scan(&ix.Name, &ix.Kind, &ix.Primary, &ix.Unique, &ix.Constraint, &desc, &included, &col, &ix.Filter); err != nil {
			rows.Close()
			return nil, err
		}
		ix.Constraint = ix.Constraint || ix.Primary
		ix.Kind = strings.ReplaceAll(ix.Kind, "_", " ")
		n := len(info.Indexes)
		if n == 0 || info.Indexes[n-1].Name != ix.Name {
			info.Indexes = append(info.Indexes, ix)
			n++
		}
		last := &info.Indexes[n-1]
		if included {
			last.Include = append(last.Include, col)
		} else {
			last.Keys = append(last.Keys, col)
			last.Desc = append(last.Desc, desc)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT fk.name, fk.is_system_named, pc.name, rs.name, rt.name, rc.name,
			fk.delete_referential_action_desc, fk.update_referential_action_desc
		FROM sys.foreign_keys fk
		JOIN sys.foreign_key_columns fkc ON fkc.constraint_object_id = fk.object_id
		JOIN sys.columns pc ON pc.object_id = fkc.parent_object_id AND pc.column_id = fkc.parent_column_id
		JOIN sys.tables rt ON rt.object_id = fk.referenced_object_id
		JOIN sys.schemas rs ON rs.schema_id = rt.schema_id
		JOIN sys.columns rc ON rc.object_id = fkc.referenced_object_id AND rc.column_id = fkc.referenced_column_id
		WHERE fk.parent_object_id = OBJECT_ID(@p1)
		ORDER BY fk.name, fkc.constraint_column_id`, name)
	if err != nil {
		return nil, err
	}
	lastName := ""
	for rows.Next() {
		var fkName, col, refSchema, refTable, refCol, onDelete, onUpdate string
		var system bool
		if err := rows.Scan(&fkName, &system, &col, &refSchema, &refTable, &refCol, &onDelete, &onUpdate); err != nil {
			rows.Close()
			return nil, err
		}
		if len(info.FKs) == 0 || fkName != lastName {
			fk := foreignKey{Name: fkName, RefTable: refSchema + "." + refTable,
				OnDelete: strings.ReplaceAll(onDelete, "_", " "), OnUpdate: strings.ReplaceAll(onUpdate, "_", " ")}
			if system {
				fk.Name = ""
			}
			info.FKs = append(info.FKs, fk)
			lastName = fkName
		}
		last := &info.FKs[len(info.FKs)-1]
		last.Cols = append(last.Cols, col)
		last.RefCols = append(last.RefCols, refCol)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = db.QueryContext(ctx, `SELECT CASE WHEN is_system_named = 0 THEN name ELSE '' END, definition
		FROM sys.check_constraints WHERE parent_object_id = OBJECT_ID(@p1) ORDER BY name`, name)
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

// msTypeText writes a system type with its size, e.g. nvarchar(50), decimal(10,2), varchar(max).
func msTypeText(base string, maxLen, prec, scale int) string {
	switch base {
	case "varchar", "char", "varbinary", "binary":
		if maxLen == -1 {
			return base + "(max)"
		}
		return fmt.Sprintf("%s(%d)", base, maxLen)
	case "nvarchar", "nchar":
		if maxLen == -1 {
			return base + "(max)"
		}
		return fmt.Sprintf("%s(%d)", base, maxLen/2)
	case "decimal", "numeric":
		return fmt.Sprintf("%s(%d,%d)", base, prec, scale)
	case "datetime2", "time", "datetimeoffset":
		return fmt.Sprintf("%s(%d)", base, scale)
	case "float":
		if prec != 53 {
			return fmt.Sprintf("float(%d)", prec)
		}
	}
	return base
}

// selectAll orders by primary key, otherwise as SQL Server returns the rows.
func (e sqlServer) selectAll(t *tableInfo, page int) string {
	order := orderBy(e, t)
	if order == "" {
		order = "(SELECT NULL)"
	}
	q := "SELECT " + columnList(e, t) + " FROM " + quoted(e, t.table) + " ORDER BY " + order
	if page >= 0 {
		q += fmt.Sprintf(" OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", page*pageSize, pageSize)
	}
	return q
}

// ---- CREATE TABLE ----

// createScript writes the CREATE TABLE of the table with its constraints, then its indexes.
func (e sqlServer) createScript(t *tableInfo) string {
	name := quoted(e, t.table)
	var b strings.Builder
	b.WriteString("CREATE TABLE " + name + " (\n")
	var parts []string
	for _, c := range t.Columns {
		parts = append(parts, "    "+e.columnDef(c))
	}
	for _, ix := range t.Indexes {
		if !ix.Constraint {
			continue
		}
		kind := "UNIQUE"
		if ix.Primary {
			kind = "PRIMARY KEY"
		}
		parts = append(parts, fmt.Sprintf("    CONSTRAINT %s %s %s (%s)", e.quote(ix.Name), kind, ix.Kind, e.keyList(ix)))
	}
	for _, fk := range t.FKs {
		s := "    "
		if fk.Name != "" {
			s += "CONSTRAINT " + e.quote(fk.Name) + " "
		}
		ref, refName, _ := strings.Cut(fk.RefTable, ".")
		s += fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s.%s (%s)", quoteList(e, fk.Cols), e.quote(ref), e.quote(refName), quoteList(e, fk.RefCols))
		if fk.OnDelete != "NO ACTION" {
			s += " ON DELETE " + fk.OnDelete
		}
		if fk.OnUpdate != "NO ACTION" {
			s += " ON UPDATE " + fk.OnUpdate
		}
		parts = append(parts, s)
	}
	for _, c := range t.Checks {
		s := "    "
		if c.Name != "" {
			s += "CONSTRAINT " + e.quote(c.Name) + " "
		}
		parts = append(parts, s+"CHECK "+c.Def)
	}
	b.WriteString(strings.Join(parts, ",\n"))
	b.WriteString("\n);\n")

	for _, ix := range t.Indexes {
		if ix.Constraint {
			continue
		}
		switch ix.Kind {
		case "CLUSTERED COLUMNSTORE":
			fmt.Fprintf(&b, "CREATE CLUSTERED COLUMNSTORE INDEX %s ON %s;\n", e.quote(ix.Name), name)
			continue
		case "NONCLUSTERED COLUMNSTORE":
			fmt.Fprintf(&b, "CREATE NONCLUSTERED COLUMNSTORE INDEX %s ON %s (%s);\n", e.quote(ix.Name), name, quoteList(e, ix.Include))
			continue
		case "CLUSTERED", "NONCLUSTERED":
		default:
			fmt.Fprintf(&b, "-- %s index %s is not written here\n", ix.Kind, e.quote(ix.Name))
			continue
		}
		unique := ""
		if ix.Unique {
			unique = "UNIQUE "
		}
		fmt.Fprintf(&b, "CREATE %s%s INDEX %s ON %s (%s)", unique, ix.Kind, e.quote(ix.Name), name, e.keyList(ix))
		if len(ix.Include) > 0 {
			b.WriteString(" INCLUDE (" + quoteList(e, ix.Include) + ")")
		}
		if ix.Filter != "" {
			b.WriteString(" WHERE " + ix.Filter)
		}
		b.WriteString(";\n")
	}
	return b.String()
}

func (e sqlServer) columnDef(c column) string {
	if c.Computed != "" {
		s := e.quote(c.Name) + " AS " + c.Computed
		if c.Persisted {
			s += " PERSISTED"
		}
		return s
	}
	s := e.quote(c.Name) + " " + c.Type
	if c.Identity {
		s += fmt.Sprintf(" IDENTITY(%s,%s)", c.Seed, c.Increment)
	}
	if c.Nullable {
		s += " NULL"
	} else {
		s += " NOT NULL"
	}
	if c.Default != "" {
		if c.DefName != "" {
			s += " CONSTRAINT " + e.quote(c.DefName)
		}
		s += " DEFAULT " + c.Default
	}
	return s
}

func (e sqlServer) keyList(ix index) string {
	keys := make([]string, len(ix.Keys))
	for i, k := range ix.Keys {
		keys[i] = e.quote(k)
		if ix.Desc[i] {
			keys[i] += " DESC"
		}
	}
	return strings.Join(keys, ", ")
}

// ---- Values ----

// text: bit as 1/0, dates in ISO form, binary as 0x…, uniqueidentifier as text.
func (sqlServer) text(v any, base string) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", true
	case bool:
		if x {
			return "1", false
		}
		return "0", false
	case int64:
		return strconv.FormatInt(x, 10), false
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), false
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32), false
	case string:
		return x, false
	case time.Time:
		return msTimeText(x, base), false
	case []byte:
		switch base {
		case "decimal", "numeric", "money", "smallmoney":
			return string(x), false
		case "uniqueidentifier":
			var u mssql.UniqueIdentifier
			if err := u.Scan(x); err == nil {
				return u.String(), false
			}
		}
		return "0x" + strings.ToUpper(hex.EncodeToString(x)), false
	}
	return fmt.Sprint(v), false
}

// msTimeText writes a date or time in ISO form, with only the parts the type has.
func msTimeText(t time.Time, base string) string {
	switch base {
	case "date":
		return t.Format("2006-01-02")
	case "time":
		return t.Format("15:04:05.9999999")
	case "smalldatetime":
		return t.Format("2006-01-02T15:04:05")
	case "datetime":
		return t.Format("2006-01-02T15:04:05.000")
	case "datetimeoffset":
		return t.Format("2006-01-02T15:04:05.9999999-07:00")
	}
	return t.Format("2006-01-02T15:04:05.9999999")
}

func (e sqlServer) literal(v any, base string) string {
	s, null := e.text(v, base)
	if null {
		return "NULL"
	}
	switch v.(type) {
	case bool, int64, float64, float32:
		return s
	case string:
		return "N" + sqlString(s)
	case []byte:
		if base != "uniqueidentifier" {
			return s // a number (decimal, money) or binary as 0x…
		}
	}
	return sqlString(s)
}

// ---- INSERT scripts ----

func (e sqlServer) dropScript(t *tableInfo) string {
	name := quoted(e, t.table)
	return fmt.Sprintf("IF OBJECT_ID(N%s, N'U') IS NOT NULL DROP TABLE %s;", sqlString(name), name)
}

func (e sqlServer) insertPrefix(t *tableInfo, cols []string) string {
	return "INSERT INTO " + quoted(e, t.table) + " (" + strings.Join(cols, ", ") + ") VALUES ("
}

func hasIdentity(t *tableInfo) bool {
	for _, c := range t.Columns {
		if c.Identity {
			return true
		}
	}
	return false
}

// Identity columns take the values of the script only with IDENTITY_INSERT on.
func (e sqlServer) beforeInserts(t *tableInfo) string {
	if !hasIdentity(t) {
		return ""
	}
	return "SET IDENTITY_INSERT " + quoted(e, t.table) + " ON;\n"
}

func (e sqlServer) afterInserts(t *tableInfo) string {
	if !hasIdentity(t) {
		return ""
	}
	return "SET IDENTITY_INSERT " + quoted(e, t.table) + " OFF;\n"
}

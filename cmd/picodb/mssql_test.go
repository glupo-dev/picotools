package main

import (
	"testing"
	"time"
)

func orders() *tableInfo {
	return &tableInfo{
		table: table{Schema: "dbo", Name: "Orders", Rows: 12345},
		Columns: []column{
			{Name: "OrderId", Type: "int", Base: "int", Identity: true, Seed: "1", Increment: "1"},
			{Name: "CustomerId", Type: "int", Base: "int"},
			{Name: "Total", Type: "decimal(10,2)", Base: "decimal", Nullable: true, Default: "((0))", DefName: "DF_Orders_Total"},
			{Name: "Notes", Type: "nvarchar(500)", Base: "nvarchar", Nullable: true},
			{Name: "Twice", Base: "decimal", Computed: "([Total]*(2))", ReadOnly: true},
		},
		Indexes: []index{
			{Name: "PK_Orders", Kind: "CLUSTERED", Primary: true, Unique: true, Constraint: true, Keys: []string{"OrderId"}, Desc: []bool{false}},
			{Name: "IX_Orders_Customer", Kind: "NONCLUSTERED", Keys: []string{"CustomerId"}, Desc: []bool{true}, Include: []string{"Total"}},
		},
		FKs: []foreignKey{{Name: "FK_Orders_Customers", Cols: []string{"CustomerId"}, RefTable: "dbo.Customers", RefCols: []string{"CustomerId"}, OnDelete: "CASCADE", OnUpdate: "NO ACTION"}},
	}
}

func TestCreateScript(t *testing.T) {
	want := `CREATE TABLE [dbo].[Orders] (
    [OrderId] int IDENTITY(1,1) NOT NULL,
    [CustomerId] int NOT NULL,
    [Total] decimal(10,2) NULL CONSTRAINT [DF_Orders_Total] DEFAULT ((0)),
    [Notes] nvarchar(500) NULL,
    [Twice] AS ([Total]*(2)),
    CONSTRAINT [PK_Orders] PRIMARY KEY CLUSTERED ([OrderId]),
    CONSTRAINT [FK_Orders_Customers] FOREIGN KEY ([CustomerId]) REFERENCES [dbo].[Customers] ([CustomerId]) ON DELETE CASCADE
);
CREATE NONCLUSTERED INDEX [IX_Orders_Customer] ON [dbo].[Orders] ([CustomerId] DESC) INCLUDE ([Total]);
`
	if got := (sqlServer{}).createScript(orders()); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSelectAll(t *testing.T) {
	want := "SELECT [OrderId], [CustomerId], [Total], [Notes], [Twice] FROM [dbo].[Orders] ORDER BY [OrderId] OFFSET 100 ROWS FETCH NEXT 50 ROWS ONLY"
	if got := (sqlServer{}).selectAll(orders(), 2); got != want {
		t.Errorf("got %s", got)
	}
	o := orders()
	o.Indexes = nil
	want = "SELECT [OrderId], [CustomerId], [Total], [Notes], [Twice] FROM [dbo].[Orders] ORDER BY (SELECT NULL)"
	if got := (sqlServer{}).selectAll(o, -1); got != want {
		t.Errorf("got %s", got)
	}
}

func TestTypeText(t *testing.T) {
	cases := []struct {
		base             string
		max, prec, scale int
		want             string
	}{
		{"nvarchar", 100, 0, 0, "nvarchar(50)"},
		{"nvarchar", -1, 0, 0, "nvarchar(max)"},
		{"varchar", 20, 0, 0, "varchar(20)"},
		{"decimal", 9, 10, 2, "decimal(10,2)"},
		{"datetime2", 8, 27, 7, "datetime2(7)"},
		{"float", 8, 53, 0, "float"},
		{"int", 4, 10, 0, "int"},
	}
	for _, c := range cases {
		if got := msTypeText(c.base, c.max, c.prec, c.scale); got != c.want {
			t.Errorf("%s: got %s, want %s", c.base, got, c.want)
		}
	}
}

func TestLiteral(t *testing.T) {
	when := time.Date(2026, 1, 3, 10, 15, 0, 0, time.UTC)
	cases := []struct {
		v    any
		base string
		want string
	}{
		{nil, "int", "NULL"},
		{int64(42), "int", "42"},
		{true, "bit", "1"},
		{"it's", "nvarchar", "N'it''s'"},
		{[]byte("120.50"), "decimal", "120.50"},
		{[]byte{0xCA, 0xFE}, "varbinary", "0xCAFE"},
		{when, "datetime", "'2026-01-03T10:15:00.000'"},
		{when, "date", "'2026-01-03'"},
	}
	for _, c := range cases {
		if got := (sqlServer{}).literal(c.v, c.base); got != c.want {
			t.Errorf("%v (%s): got %s, want %s", c.v, c.base, got, c.want)
		}
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 12345: "12,345", 1234567: "1,234,567"} {
		if got := thousands(n); got != want {
			t.Errorf("%d: got %s", n, got)
		}
	}
}

func TestPickInLongList(t *testing.T) {
	m := newModel(&Store{})
	cursor := 0
	m.pick('1', &cursor, 30)
	m.pick('2', &cursor, 30)
	if cursor != 11 || m.number != "12" {
		t.Errorf("cursor %d, number %q", cursor, m.number)
	}
	m.pick('9', &cursor, 30) // 129 does not exist: ignored
	if cursor != 11 || m.number != "12" {
		t.Errorf("cursor %d, number %q", cursor, m.number)
	}
	if i, ok := m.pick('3', &cursor, 5); !ok || i != 2 {
		t.Errorf("short list: %d %v", i, ok)
	}
}

func TestLocalDBPipe(t *testing.T) {
	info := "Name:               MSSQLLocalDB\r\nVersion:            15.0.4153.1\r\nStatus:             Wird ausgeführt\r\nInstanz-Pipename:   np:\\\\.\\pipe\\LOCALDB#5E8D3C2A\\tsql\\query\r\n"
	if got, want := localDBPipe(info), `np:\\.\pipe\LOCALDB#5E8D3C2A\tsql\query`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := localDBPipe("Status: Stopped\r\nInstance pipe name: \r\n"); got != "" {
		t.Errorf("stopped instance: got %q", got)
	}
}

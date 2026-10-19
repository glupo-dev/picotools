package main

// SQL Server LocalDB: Server=(localdb)\<instance>. LocalDB has no TCP port, only a named
// pipe whose name changes at each start, so it is asked to SqlLocalDB.exe (Windows only).

import (
	"regexp"
	"strings"

	"github.com/microsoft/go-mssqldb/msdsn"
)

func isLocalDB(cfg msdsn.Config) bool { return strings.EqualFold(cfg.Host, "(localdb)") }

// localDBPipe finds the pipe in the output of "SqlLocalDB info <instance>", e.g.
// np:\\.\pipe\LOCALDB#5E8D3C2A\tsql\query. The labels are translated, the pipe is not.
func localDBPipe(info string) string {
	return regexp.MustCompile(`np:\\\\\.\\pipe\\\S+`).FindString(info)
}

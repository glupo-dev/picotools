//go:build !windows

package main

import (
	"errors"

	"github.com/microsoft/go-mssqldb/msdsn"
)

func useLocalDB(cfg *msdsn.Config) error {
	if isLocalDB(*cfg) {
		return errors.New("SQL Server LocalDB is only available on Windows")
	}
	return nil
}

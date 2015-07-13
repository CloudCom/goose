// +build !nosqlite

package main

// including go-sqlite3
import _ "github.com/mattn/go-sqlite3"

func init() {
	drivers = append(drivers, "sqlite")
}

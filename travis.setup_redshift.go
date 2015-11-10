// +build travis

package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	var cmd string
	if len(os.Args) == 2 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "setup":
		setup()
	case "destroy":
		destroy()
	default:
		fmt.Fprintf(os.Stderr, "usage: %s {setup|destroy}\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}
}

func setup() {
	dsn := os.Getenv("REDSHIFT_DATABASE_DSN")

	newdb := "goose-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	var newDSN string

	if strings.HasPrefix(dsn, "postgres://") {
		u, err := url.Parse(dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not parse $REDSHIFT_DATABASE_DSN\n")
			os.Exit(1)
		}
		u.Path = newdb
		newDSN = u.String()
	} else {
		foundDBName := false
		for _, f := range strings.Fields(dsn) {
			kv := strings.SplitN(f, "=", 2)
			if kv[0] == "dbname" {
				f = "dbname=" + newdb
				foundDBName = true
			}
			newDSN = newDSN + f + " "
		}
		if !foundDBName {
			newDSN = newDSN + "dbname=" + newdb
		}
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not connect to Redshift: %s\n", err)
		os.Exit(1)
	}
	_, err = db.Exec(fmt.Sprintf("create database %q", newdb))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create database: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("REDSHIFT_DATABASE_DSN=%s\n", newDSN)
	os.Exit(0)
}

func destroy() {
	dsn := os.Getenv("REDSHIFT_DATABASE_DSN")

	var dbname string

	if strings.HasPrefix(dsn, "postgres://") {
		u, err := url.Parse(dsn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not parse $REDSHIFT_DATABASE_DSN\n")
			os.Exit(1)
		}
		dbname = u.Path
	} else {
		for _, f := range strings.Fields(dsn) {
			kv := strings.SplitN(f, "=", 2)
			if len(kv) != 2 || kv[0] == "dbname" {
				dbname = kv[1]
				break
			}
		}
	}
	if dbname == "" {
		fmt.Fprintf(os.Stderr, "Could not find db name in $REDSHIFT_DATABASE_DSN\n")
		os.Exit(1)
	}

	db, err := sql.Open("postgres", strings.Replace(dsn, dbname, "dev", 1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not connect to Redshift: %s\n", err)
		os.Exit(1)
	}
	_, err = db.Exec(fmt.Sprintf("drop database %q", dbname))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not drop database: %s\n", err)
		os.Exit(1)
	}
}

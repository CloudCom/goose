package main

import (
	"database/sql"
	"fmt"
	"github.com/cloudcom/goose/lib/goose"
	"log"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

var statusCmd = &Command{
	Name:    "status",
	Usage:   "",
	Summary: "dump the migration status for the current DB",
	Help:    `status extended help here...`,
	Run:     statusRun,
}

type StatusData struct {
	Source string
	Status string
}

func statusRun(cmd *Command, args ...string) {

	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	// collect all migrations
	min := int64(0)
	max := int64((1 << 63) - 1)
	migrations, e := goose.CollectMigrations(conf.MigrationsDir, min, max)
	if e != nil {
		log.Fatal(e)
	}

	// we depend on time parsing, so make sure it's enabled with the mysql driver
	if conf.Driver.Name == "mysql" {
		i := strings.Index(conf.Driver.OpenStr, "?")
		if i == -1 {
			i = len(conf.Driver.OpenStr)
			conf.Driver.OpenStr = conf.Driver.OpenStr + "?"
		}
		i++

		q, err := url.ParseQuery(conf.Driver.OpenStr[i:])
		if err != nil {
			log.Fatal(err)
		}
		q.Set("parseTime", "true")

		conf.Driver.OpenStr = conf.Driver.OpenStr[:i] + q.Encode()
	}

	db, e := goose.OpenDBFromDBConf(conf)
	if e != nil {
		log.Fatal("couldn't open DB:", e)
	}
	defer db.Close()

	// must ensure that the version table exists if we're running on a pristine DB
	if _, e := goose.EnsureDBVersion(conf, db); e != nil {
		log.Fatal(e)
	}

	fmt.Printf("goose: status for environment '%v'\n", conf.Env)
	fmt.Println("    Applied At                  Migration")
	fmt.Println("    =======================================")
	for _, m := range migrations {
		printMigrationStatus(db, m.Version, filepath.Base(m.Source))
	}
}

func printMigrationStatus(db *sql.DB, version int64, script string) {
	var row goose.MigrationRecord
	q := "SELECT tstamp, is_applied FROM goose_db_version WHERE version_id=? ORDER BY tstamp DESC LIMIT 1"
	e := db.QueryRow(q, version).Scan(&row.TStamp, &row.IsApplied)

	if e != nil && e != sql.ErrNoRows {
		log.Fatal(e)
	}

	var appliedAt string

	if row.IsApplied {
		appliedAt = row.TStamp.Format(time.ANSIC)
	} else {
		appliedAt = "Pending"
	}

	fmt.Printf("    %-24s -- %v\n", appliedAt, script)
}

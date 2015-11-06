package goose

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

var (
	ErrTableDoesNotExist = errors.New("table does not exist")
	ErrNoPreviousVersion = errors.New("no previous version found")
)

type Direction bool

func (d Direction) String() string {
	if d == DirectionUp {
		return "up"
	} else {
		return "down"
	}
}

const (
	DirectionDown = Direction(false)
	DirectionUp   = Direction(true)
)

//TODO these need to be in cmd, not lib
var templates = template.Must(template.ParseGlob(filepath.Join(templatesDir(), "*")))
var goMigrationDriverTemplate = templates.Lookup("migration-main.go.tmpl")
var goMigrationTemplate = templates.Lookup("migration.go.tmpl")
var sqlMigrationTemplate = templates.Lookup("migration.sql.tmpl")

func templatesDir() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		return filepath.Join(filepath.Dir(file), "templates")
	}
	// runtime.Caller() failed. That's weird...
	// Try `./templates`, we'll either get lucky, or `template.Must()` will blow
	// up for us.
	return "./templates"
}

type Migration struct {
	Version   int64
	IsApplied bool
	TStamp    time.Time
	Source    string // path to .go or .sql script
}

type migrationSorter []*Migration

// helpers so we can use pkg sort
func (ms migrationSorter) Len() int           { return len(ms) }
func (ms migrationSorter) Swap(i, j int)      { ms[i], ms[j] = ms[j], ms[i] }
func (ms migrationSorter) Less(i, j int) bool { return ms[i].Version < ms[j].Version }

func RunMigrations(conf *DBConf, migrationsDir string, target int64) (err error) {
	db, err := OpenDBFromDBConf(conf)
	if err != nil {
		return err
	}
	defer db.Close()

	return RunMigrationsOnDb(conf, migrationsDir, target, db)
}

// Runs migration on a specific database instance.
func RunMigrationsOnDb(conf *DBConf, migrationsDir string, target int64, db *sql.DB) (err error) {
	//TODO get rid of migrationsDir, it's already in conf.MigrationsDir
	current, err := EnsureDBVersion(conf, db)
	if err != nil {
		return err
	}

	migrations, err := CollectMigrations(migrationsDir)
	if err != nil {
		return err
	}

	if err := getMigrationsStatus(conf, db, migrations); err != nil {
		return err
	}

	direction := DirectionUp
	if target < current {
		direction = DirectionDown
	}

	var neededMigrations []*Migration
	for _, m := range migrations {
		if direction == DirectionUp {
			if m.Version > target {
				continue
			}
			if m.IsApplied {
				continue
			}
		} else {
			if m.Version < target {
				continue
			}
			if !m.IsApplied {
				continue
			}
		}
		neededMigrations = append(neededMigrations, m)
	}

	if len(neededMigrations) == 0 {
		fmt.Printf("goose: no migrations to run. current version: %d, target: %d\n", current, target)
		return nil
	}

	fmt.Printf("goose: migrating db environment '%v', current version: %d, target: %d\n",
		conf.Env, current, target)

	ms := migrationSorter(neededMigrations)
	if direction == DirectionUp {
		sort.Sort(ms)
	} else {
		sort.Sort(sort.Reverse(ms))
	}

	for _, m := range ms {
		switch filepath.Ext(m.Source) {
		case ".go":
			err = runGoMigration(conf, m.Source, m.Version, direction)
		case ".sql":
			err = runSQLMigration(conf, db, m.Source, m.Version, direction)
		}

		if err != nil {
			return errors.New(fmt.Sprintf("FAIL %v, quitting migration", err))
		}

		fmt.Println("OK   ", filepath.Base(m.Source))
	}

	return nil
}

// collect all the valid looking migration scripts in the
// migrations folder, and key them by version
func CollectMigrations(dirpath string) (m []*Migration, err error) {
	// extract the numeric component of each migration,
	// filter out any uninteresting files,
	// and ensure we only have one file per migration version.
	filepath.Walk(dirpath, func(name string, info os.FileInfo, err error) error {

		if v, e := NumericComponent(name); e == nil {

			for _, g := range m {
				if v == g.Version {
					log.Fatalf("more than one file specifies the migration for version %d (%s and %s)",
						v, g.Source, filepath.Join(dirpath, name))
				}
			}

			m = append(m, &Migration{Version: v, Source: name})
		}

		return nil
	})

	return m, nil
}

// look for migration scripts with names in the form:
//  XXX_descriptivename.ext
// where XXX specifies the version number
// and ext specifies the type of migration
func NumericComponent(name string) (int64, error) {
	base := filepath.Base(name)

	if ext := filepath.Ext(base); ext != ".go" && ext != ".sql" {
		return 0, errors.New("not a recognized migration file type")
	}

	idx := strings.Index(base, "_")
	if idx < 0 {
		return 0, errors.New("no separator found")
	}

	n, e := strconv.ParseInt(base[:idx], 10, 64)
	if e == nil && n <= 0 {
		return 0, errors.New("migration IDs must be greater than zero")
	}

	return n, e
}

func getMigrationsStatus(conf *DBConf, db *sql.DB, migrations []*Migration) error {
	rows, err := conf.Driver.Dialect.dbVersionQuery(db)
	if err != nil {
		if err == ErrTableDoesNotExist {
			for _, m := range migrations {
				m.IsApplied = false
			}
			return nil
		}
		return fmt.Errorf("getting db version: %s", err)
	}
	defer rows.Close()

	mm := map[int64]*Migration{}
	for _, m := range migrations {
		mm[m.Version] = m
		// default to false so if the DB doesn't know about the migration...
		m.IsApplied = false
	}

	for rows.Next() {
		var row Migration
		if err = rows.Scan(&row.Version, &row.IsApplied, &row.TStamp); err != nil {
			log.Fatal("error scanning rows:", err)
		}

		m, ok := mm[row.Version]
		if !ok {
			continue
		}
		if !row.TStamp.After(m.TStamp) {
			// If the migration went up, then down, it'll have multiple rows.
			// But we only want the newest, so skip this row if it's older.
			continue
		}
		m.IsApplied = row.IsApplied
		m.TStamp = row.TStamp
	}

	return nil
}

// retrieve the current version for this DB.
// Create and initialize the DB version table if it doesn't exist.
func EnsureDBVersion(conf *DBConf, db *sql.DB) (int64, error) {
	rows, err := conf.Driver.Dialect.dbVersionQuery(db)
	if err != nil {
		if err == ErrTableDoesNotExist {
			return 0, createVersionTable(conf, db)
		}
		return 0, fmt.Errorf("getting db version: %#v", err)
	}
	defer rows.Close()

	// The most recent record for each migration specifies
	// whether it has been applied or rolled back.
	// The first version we find that has been applied is the current version.

	toSkip := make([]int64, 0)

	for rows.Next() {
		var row Migration
		if err = rows.Scan(&row.Version, &row.IsApplied, &row.TStamp); err != nil {
			log.Fatal("error scanning rows:", err)
		}

		// have we already marked this version to be skipped?
		skip := false
		for _, v := range toSkip {
			if v == row.Version {
				skip = true
				break
			}
		}

		if skip {
			continue
		}

		// if version has been applied we're done
		if row.IsApplied {
			return row.Version, nil
		}

		// latest version of migration has not been applied.
		toSkip = append(toSkip, row.Version)
	}

	panic("failure in EnsureDBVersion()")
}

// Create the goose_db_version table
// and insert the initial 0 value into it
func createVersionTable(conf *DBConf, db *sql.DB) error {
	txn, err := db.Begin()
	if err != nil {
		return err
	}

	d := conf.Driver.Dialect

	if _, err := txn.Exec(d.createVersionTableSql()); err != nil {
		txn.Rollback()
		return fmt.Errorf("creating migration table: %s", err)
	}

	version := 0
	applied := true
	if _, err := txn.Exec(d.insertVersionSql(), version, applied); err != nil {
		txn.Rollback()
		return fmt.Errorf("inserting first migration: %s", err)
	}

	return txn.Commit()
}

// wrapper for EnsureDBVersion for callers that don't already have
// their own DB instance
func GetDBVersion(conf *DBConf) (version int64, err error) {
	db, err := OpenDBFromDBConf(conf)
	if err != nil {
		return -1, err
	}
	defer db.Close()

	version, err = EnsureDBVersion(conf, db)
	if err != nil {
		return -1, err
	}

	return version, nil
}

func GetPreviousDBVersion(dirpath string, version int64) (previous int64, err error) {
	previous = -1
	sawGivenVersion := false

	filepath.Walk(dirpath, func(name string, info os.FileInfo, walkerr error) error {

		if !info.IsDir() {
			if v, e := NumericComponent(name); e == nil {
				if v > previous && v < version {
					previous = v
				}
				if v == version {
					sawGivenVersion = true
				}
			}
		}

		return nil
	})

	if previous == -1 {
		if sawGivenVersion {
			// the given version is (likely) valid but we didn't find
			// anything before it.
			// 'previous' must reflect that no migrations have been applied.
			previous = 0
		} else {
			err = ErrNoPreviousVersion
		}
	}

	return
}

// helper to identify the most recent possible version
// within a folder of migration scripts
func GetMostRecentDBVersion(dirpath string) (version int64, err error) {
	version = -1

	filepath.Walk(dirpath, func(name string, info os.FileInfo, walkerr error) error {
		if walkerr != nil {
			return walkerr
		}

		if !info.IsDir() {
			if v, e := NumericComponent(name); e == nil {
				if v > version {
					version = v
				}
			}
		}

		return nil
	})

	if version == -1 {
		err = errors.New("no valid version found")
	}

	return
}

func CreateMigration(name, migrationType, dir string, t time.Time) (path string, err error) {
	if migrationType != "go" && migrationType != "sql" {
		return "", errors.New("migration type must be 'go' or 'sql'")
	}

	timestamp := t.Format("20060102150405")
	filename := fmt.Sprintf("%v_%v.%v", timestamp, name, migrationType)

	fpath := filepath.Join(dir, filename)

	var tmpl *template.Template
	if migrationType == "sql" {
		tmpl = sqlMigrationTemplate
	} else {
		tmpl = goMigrationTemplate
	}

	path, err = writeTemplateToFile(fpath, tmpl, timestamp)

	return
}

// Update the version table for the given migration,
// and finalize the transaction.
func FinalizeMigration(conf *DBConf, txn *sql.Tx, direction Direction, v int64) error {
	// XXX: drop goose_db_version table on some minimum version number?
	stmt := conf.Driver.Dialect.insertVersionSql()
	if _, err := txn.Exec(stmt, v, bool(direction)); err != nil {
		txn.Rollback()
		return err
	}

	return txn.Commit()
}

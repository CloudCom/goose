package goose

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/kylelemons/go-gypsy/yaml"
	"github.com/lib/pq"
)

// DBDriver encapsulates the info needed to work with
// a specific database driver
type DBDriver struct {
	Name    string
	OpenStr string
	Import  string
	Dialect SqlDialect
}

type DBConf struct {
	MigrationsDir string
	Env           string
	Driver        DBDriver
}

// findDBConf looks for a dbconf.yaml file starting at the given directory and
// walking up in the directory hierarchy.
// Returns empty string if not found.
func findDBConf(dbDir string) string {
	dbDir, err := filepath.Abs(dbDir)
	if err != nil {
		return ""
	}

	for {
		path := filepath.Join(dbDir, "dbconf.yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
		path = filepath.Join(dbDir, "dbconf.yml")
		if _, err := os.Stat(path); err == nil {
			return path
		}

		nextDir := filepath.Dir(dbDir)
		if nextDir == dbDir {
			// at the root
			break
		}
		dbDir = nextDir
	}

	return ""
}

// extract configuration details from the given file
func NewDBConf(dbDir, env string) (*DBConf, error) {
	cfgFile := findDBConf(dbDir)
	if cfgFile == "" {
		return nil, fmt.Errorf("could not find dbconf.yaml")
	}
	dbDir = filepath.Dir(cfgFile)
	migrationsDir := filepath.Join(dbDir, "migrations")

	f, err := yaml.ReadFile(cfgFile)
	if err != nil {
		return nil, err
	}

	if md, err := f.Get(fmt.Sprintf("%s.migrationsDir", env)); err == nil {
		if filepath.IsAbs(md) {
			migrationsDir = md
		} else {
			migrationsDir = filepath.Join(dbDir, md)
		}
	}

	drv, err := f.Get(fmt.Sprintf("%s.driver", env))
	if err != nil {
		return nil, err
	}
	drv = os.ExpandEnv(drv)

	open, err := f.Get(fmt.Sprintf("%s.open", env))
	if err != nil {
		return nil, err
	}
	open = os.ExpandEnv(open)

	// Automatically parse postgres urls
	if drv == "postgres" {

		// Assumption: If we can parse the URL, we should
		if parsedURL, err := pq.ParseURL(open); err == nil && parsedURL != "" {
			open = parsedURL
		}
	}

	d := newDBDriver(drv, open)

	// allow the configuration to override the Import for this driver
	if imprt, err := f.Get(fmt.Sprintf("%s.import", env)); err == nil {
		d.Import = imprt
	}

	// allow the configuration to override the Dialect for this driver
	if dialect, err := f.Get(fmt.Sprintf("%s.dialect", env)); err == nil {
		d.Dialect = dialectByName(dialect)
	}

	if !d.IsValid() {
		return nil, errors.New(fmt.Sprintf("Invalid DBConf: %v", d))
	}

	return &DBConf{
		MigrationsDir: migrationsDir,
		Env:           env,
		Driver:        d,
	}, nil
}

// Create a new DBDriver and populate driver specific
// fields for drivers that we know about.
// Further customization may be done in NewDBConf
func newDBDriver(name, open string) DBDriver {

	d := DBDriver{
		Name:    name,
		OpenStr: open,
	}

	switch name {
	case "postgres":
		d.Import = "github.com/lib/pq"
		d.Dialect = &PostgresDialect{}

	case "redshift":
		d.Name = "postgres"
		d.Import = "github.com/lib/pq"
		d.Dialect = &RedshiftDialect{}

	case "mymysql":
		d.Import = "github.com/ziutek/mymysql/godrv"
		d.Dialect = &MySqlDialect{}

	case "mysql":
		d.Import = "github.com/go-sql-driver/mysql"
		d.Dialect = &MySqlDialect{}

	case "sqlite3":
		d.Import = "github.com/mattn/go-sqlite3"
		d.Dialect = &Sqlite3Dialect{}
	}

	return d
}

// ensure we have enough info about this driver
func (drv *DBDriver) IsValid() bool {
	return len(drv.Import) > 0 && drv.Dialect != nil
}

// OpenDBFromDBConf wraps database/sql.DB.Open() and configures
// the newly opened DB based on the given DBConf.
//
// Callers must Close() the returned DB.
func OpenDBFromDBConf(conf *DBConf) (*sql.DB, error) {
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
			return nil, err
		}
		q.Set("parseTime", "true")

		conf.Driver.OpenStr = conf.Driver.OpenStr[:i] + q.Encode()
	}

	return sql.Open(conf.Driver.Name, conf.Driver.OpenStr)
}

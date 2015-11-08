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
	Driver        DBDriver
}

var defaultDBConfYaml = `
migrationsDir: $DB_MIGRATIONS_DIR
driver: $DB_DRIVER
import: $DB_DRIVER_IMPORT
dialect: $DB_DIALECT
open: $DB_DSN
`

// findDBConf looks for a dbconf.yaml file starting at the given directory and
// walking up in the directory hierarchy.
// Returns empty string if not found.
func findDBConf(dbDir string) string {
	dbDir, err := filepath.Abs(dbDir)
	if err != nil {
		return ""
	}

	for {
		paths := []string{
			"dbconf.yaml",
			"dbconf.yml",
			filepath.Join("db", "dbconf.yaml"),
			filepath.Join("db", "dbconf.yml"),
		}

		for _, path := range paths {
			path = filepath.Join(dbDir, path)
			if _, err := os.Stat(path); err == nil {
				return path
			}
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

func confGet(f *yaml.File, env string, name string) (string, error) {
	if env != "" {
		if v, err := f.Get(fmt.Sprintf("%s.%s", env, name)); err == nil {
			return os.ExpandEnv(v), nil
		}
	}
	v, err := f.Get(name)
	if err != nil {
		return "", err
	}
	return os.ExpandEnv(v), nil
}

// extract configuration details from the given file
func NewDBConf(dbDir, env string) (*DBConf, error) {
	cfgFile := findDBConf(dbDir)
	var f *yaml.File
	if cfgFile == "" {
		root, _ := yaml.Parse(strings.NewReader(defaultDBConfYaml))
		f = &yaml.File{
			Root: root,
		}
	} else {
		dbDir = filepath.Dir(cfgFile)

		var err error
		f, err = yaml.ReadFile(cfgFile)
		if err != nil {
			return nil, fmt.Errorf("error loading config file: %s", err)
		}
	}

	migrationsDir := filepath.Join(dbDir, "migrations")
	if md, err := confGet(f, env, "migrationsDir"); err == nil {
		if filepath.IsAbs(md) {
			migrationsDir = md
		} else {
			migrationsDir = filepath.Join(dbDir, md)
		}
	}

	drv, err := confGet(f, env, "driver")
	if err != nil {
		return nil, err
	}
	var imprt string
	// see if "driver" param is a full import path
	if i := strings.LastIndex(drv, "/"); i != -1 {
		imprt = drv
		drv = imprt[i+1:]
	}

	open, _ := confGet(f, env, "open")

	d := newDBDriver(drv, open)

	if imprt != "" {
		d.Import = imprt
	}
	// allow the configuration to override the Import for this driver
	if imprt, err := confGet(f, env, "import"); err == nil && imprt != "" {
		d.Import = imprt
	}

	// allow the configuration to override the Dialect for this driver
	if dialect, err := confGet(f, env, "dialect"); err == nil && dialect != "" {
		d.Dialect = dialectByName(dialect)
	}

	if !d.IsValid() {
		return nil, errors.New(fmt.Sprintf("Invalid DBConf: %v", d))
	}

	return &DBConf{
		MigrationsDir: migrationsDir,
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

	switch strings.ToLower(name) {
	case "postgres":
		d.Name = "postgres"
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
		d.Name = "sqlite3"
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

package goose

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupDBConf(t *testing.T, confPath string, extraPath string) (string, string, func()) {
	td, err := ioutil.TempDir("", "goose-test")
	require.NoError(t, err)
	defer func() {
		if t.Failed() {
			os.RemoveAll(td)
		}
	}()

	confPath = filepath.Join(strings.Split(confPath, "/")...)
	confPath = filepath.Join(td, confPath)
	confDir := filepath.Dir(confPath)
	err = os.MkdirAll(confDir, 0700)
	require.NoError(t, err)

	err = ioutil.WriteFile(confPath,
		[]byte("\n"),
		0600)
	require.NoError(t, err)

	extraDir := filepath.Join(strings.Split(extraPath, "/")...)
	extraDir = filepath.Join(td, extraDir)
	err = os.MkdirAll(extraDir, 0700)
	require.NoError(t, err)

	return confPath, extraDir, func() { os.RemoveAll(td) }
}

func TestFindDBConf_confDir(t *testing.T) {
	confNames := []string{"db/dbconf.yaml", "db/dbconf.yml", "dbconf.yaml", "dbconf.yml"}
	for _, confName := range confNames {
		confPath, baseDir, clean := setupDBConf(t, confName, "")
		defer clean()

		path := findDBConf(baseDir)
		assert.Equal(t, confPath, path)
	}
}

func TestFindDBConf_deepDir(t *testing.T) {
	confPath, deepDir, clean := setupDBConf(t, "db/dbconf.yaml", "a/b/c")
	defer clean()

	path := findDBConf(deepDir)
	assert.Equal(t, confPath, path)
}

func TestFindDBConf_pwd(t *testing.T) {
	confPath, deepDir, clean := setupDBConf(t, "dbconf.yaml", "a/b/c")
	defer clean()

	pwd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(pwd)
	err = os.Chdir(deepDir)
	require.NoError(t, err)

	path := findDBConf("")
	assert.Equal(t, confPath, path)
}

func TestNewDBConf(t *testing.T) {
	confPath, migrationsDir, clean := setupDBConf(t, "dbconf.yaml", "dbstuff")
	defer clean()

	err := ioutil.WriteFile(confPath,
		[]byte(`
myenv:
	driver: mysql
	open: foo
	migrationsDir: dbstuff
`),
		0700)
	require.NoError(t, err)

	dbconf, err := NewDBConf(filepath.Dir(confPath), "myenv")
	require.NoError(t, err)

	assert.Equal(t, migrationsDir, dbconf.MigrationsDir)
	assert.Equal(t, "mysql", dbconf.Driver.Name)
	assert.Equal(t, "foo", dbconf.Driver.OpenStr)
}

func TestNewDBConf_default(t *testing.T) {
	// Since the default uses env vars, and also no environment, this tests
	// these 2 additional configurations as well.
	defer os.Setenv("DB_MIGRATIONS_DIR", os.Getenv("DB_MIGRATIONS_DIR"))
	os.Setenv("DB_MIGRATIONS_DIR", "/migdir")
	defer os.Setenv("DB_DRIVER", os.Getenv("DB_DRIVER"))
	os.Setenv("DB_DRIVER", "mysqlite3")
	defer os.Setenv("DB_DRIVER_IMPORT", os.Getenv("DB_DRIVER_IMPORT"))
	os.Setenv("DB_DRIVER_IMPORT", "github.com/myfork/sqlite3")
	defer os.Setenv("DB_DIALECT", os.Getenv("DB_DIALECT"))
	os.Setenv("DB_DIALECT", "sqlite3")
	defer os.Setenv("DB_DSN", os.Getenv("DB_DSN"))
	os.Setenv("DB_DSN", "foo")

	dbconf, err := NewDBConf("", "")
	require.NoError(t, err)

	assert.Equal(t, "/migdir", dbconf.MigrationsDir)
	assert.Equal(t, "mysqlite3", dbconf.Driver.Name)
	assert.Equal(t, "github.com/myfork/sqlite3", dbconf.Driver.Import)
	assert.Equal(t, &Sqlite3Dialect{}, dbconf.Driver.Dialect)
	assert.Equal(t, "foo", dbconf.Driver.OpenStr)
}

func TestNewDBConf_driverImport(t *testing.T) {
	confPath, migrationsDir, clean := setupDBConf(t, "dbconf.yaml", "migrations")
	defer clean()

	err := ioutil.WriteFile(confPath,
		[]byte(`
myenv:
	driver: github.com/myfork/mysql
	open: foo
`),
		0700)
	require.NoError(t, err)

	dbconf, err := NewDBConf(filepath.Dir(confPath), "myenv")
	require.NoError(t, err)

	assert.Equal(t, migrationsDir, dbconf.MigrationsDir)
	assert.Equal(t, "mysql", dbconf.Driver.Name)
	assert.Equal(t, "github.com/myfork/mysql", dbconf.Driver.Import)
	assert.Equal(t, &MySqlDialect{}, dbconf.Driver.Dialect)
	assert.Equal(t, "foo", dbconf.Driver.OpenStr)
}

func TestNewDBConf_driverDefaults(t *testing.T) {
	tests := []struct {
		names  []string
		driver DBDriver
	}{
		{
			[]string{"postgres", "PoStGrEs"},
			DBDriver{
				Name:    "postgres",
				Import:  "github.com/lib/pq",
				Dialect: &PostgresDialect{},
			},
		},
		{
			[]string{"redshift"},
			DBDriver{
				Name:    "postgres",
				Import:  "github.com/lib/pq",
				Dialect: &RedshiftDialect{},
			},
		},
		{
			[]string{"mymysql"},
			DBDriver{
				Name:    "mymysql",
				Import:  "github.com/ziutek/mymysql/godrv",
				Dialect: &MySqlDialect{},
			},
		},
		{
			[]string{"mysql"},
			DBDriver{
				Name:    "mysql",
				Import:  "github.com/go-sql-driver/mysql",
				Dialect: &MySqlDialect{},
			},
		},
		{
			[]string{"sqlite3"},
			DBDriver{
				Name:    "sqlite3",
				Import:  "github.com/mattn/go-sqlite3",
				Dialect: &Sqlite3Dialect{},
			},
		},
	}
	for _, test := range tests {
		for _, driverName := range test.names {
			confPath, migrationsDir, clean := setupDBConf(t, "dbconf.yaml", "migrations")
			defer clean()

			err := ioutil.WriteFile(confPath,
				[]byte(`
myenv:
	driver: `+driverName+`
`),
				0700)
			require.NoError(t, err)

			dbconf, err := NewDBConf(filepath.Dir(confPath), "myenv")
			require.NoError(t, err)

			assert.Equal(t, migrationsDir, dbconf.MigrationsDir)
			assert.Equal(t, test.driver, dbconf.Driver)
		}
	}
}

func TestImportOverride(t *testing.T) {
	dbconf, err := NewDBConf("../../_example", "customimport")
	if err != nil {
		t.Fatal(err)
	}

	got := dbconf.Driver.Import
	want := "github.com/custom/driver"
	if got != want {
		t.Errorf("bad custom import. got %v want %v", got, want)
	}
}

func TestDriverSetFromEnvironmentVariable(t *testing.T) {
	databaseUrlEnvVariableKey := "DB_DRIVER"
	databaseUrlEnvVariableVal := "sqlite3"
	databaseOpenStringKey := "DATABASE_URL"
	databaseOpenStringVal := "db.db"

	os.Setenv(databaseUrlEnvVariableKey, databaseUrlEnvVariableVal)
	os.Setenv(databaseOpenStringKey, databaseOpenStringVal)

	dbconf, err := NewDBConf("../../_example", "environment_variable_config")
	if err != nil {
		t.Fatal(err)
	}

	got := reflect.TypeOf(dbconf.Driver.Dialect)
	want := reflect.TypeOf(&Sqlite3Dialect{})

	if got != want {
		t.Errorf("Not able to read the driver type from environment variable."+
			"got %v want %v", got, want)
	}

	gotOpenString := dbconf.Driver.OpenStr
	wantOpenString := databaseOpenStringVal

	if gotOpenString != wantOpenString {
		t.Errorf("Not able to read the open string from the environment."+
			"got %v want %v", gotOpenString, wantOpenString)
	}
}

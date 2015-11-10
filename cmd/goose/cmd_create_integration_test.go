package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CloudCom/goose/lib/goose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationCreate_sql(t *testing.T) {
	td, err := ioutil.TempDir("", "goose-test-")
	require.NoError(t, err)
	defer os.RemoveAll(td)

	migrationsDir := filepath.Join(td, "migrations")
	err = os.MkdirAll(migrationsDir, 0700)
	require.NoError(t, err)

	status, out, err := run(
		[]string{"create", "-type", "sql", "mymigration"},
		map[string]string{
			"DB_DRIVER":         "sqlite3",
			"DB_MIGRATIONS_DIR": migrationsDir,
		},
	)
	require.NoError(t, err)

	assert.Equal(t, 0, status)

	require.Contains(t, out, migrationsDir)
	i := strings.Index(out, migrationsDir)
	fn := out[i:]
	fn = strings.Fields(fn)[0]

	fBS, err := ioutil.ReadFile(fn)
	require.NoError(t, err)

	tmplBS, err := goose.Asset("templates/migration.sql.tmpl")
	require.NoError(t, err)
	assert.Equal(t, string(tmplBS), string(fBS))
}

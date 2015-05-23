package goose

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

type templateData struct {
	Version    int64
	Import     string
	Conf       string // gob encoded DBConf
	Direction  bool
	Func       string
	InsertStmt string
}

func init() {
	gob.Register(PostgresDialect{})
	gob.Register(MySqlDialect{})
	gob.Register(Sqlite3Dialect{})
}

//
// Run a .go migration.
//
// In order to do this, we copy a modified version of the
// original .go migration, and execute it via `go run` along
// with a main() of our own creation.
//
func runGoMigration(conf *DBConf, path string, version int64, direction bool) error {

	// everything gets written to a temp dir, and zapped afterwards
	d, e := ioutil.TempDir("", "goose")
	if e != nil {
		log.Fatal(e)
	}
	defer os.RemoveAll(d)

	directionStr := "Down"
	if direction {
		directionStr = "Up"
	}

	var bb bytes.Buffer
	if err := gob.NewEncoder(&bb).Encode(conf); err != nil {
		return err
	}

	// XXX: there must be a better way of making this byte array
	// available to the generated code...
	// but for now, print an array literal of the gob bytes
	var sb bytes.Buffer
	sb.WriteString("[]byte{ ")
	for _, b := range bb.Bytes() {
		sb.WriteString(fmt.Sprintf("0x%02x, ", b))
	}
	sb.WriteString("}")

	td := &templateData{
		Version:    version,
		Import:     conf.Driver.Import,
		Conf:       sb.String(),
		Direction:  direction,
		Func:       fmt.Sprintf("%v_%v", directionStr, version),
		InsertStmt: conf.Driver.Dialect.insertVersionSql(),
	}
	main, e := writeTemplateToFile(filepath.Join(d, "goose_main.go"), goMigrationDriverTemplate, td)
	if e != nil {
		log.Fatal(e)
	}

	outpath := filepath.Join(d, filepath.Base(path))
	if _, e = copyFile(outpath, path); e != nil {
		log.Fatal(e)
	}

	cmd := exec.Command("go", "run", main, outpath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if e = cmd.Run(); e != nil {
		log.Fatal("`go run` failed: ", e)
	}

	return nil
}

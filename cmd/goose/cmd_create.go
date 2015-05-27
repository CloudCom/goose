package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/CloudCom/goose/lib/goose"
)

var createCmd = &Command{
	Name:    "create",
	Usage:   "<migration_name>",
	Summary: "Create the scaffolding for a new migration",
	Help:    `create extended help here...`,
	Run:     createRun,
}

var migrationType string

func init() {
	createCmd.Flag.StringVar(&migrationType, "type", "sql", "type of migration to create [sql,go]")
}

func createRun(cmd *Command, args ...string) {
	if len(args) != 1 {
		cmd.Flag.Usage()
		os.Exit(1)
	}

	conf, err := dbConfFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	if err = os.MkdirAll(conf.MigrationsDir, 0777); err != nil {
		log.Fatal(err)
	}

	n, err := goose.CreateMigration(args[0], migrationType, conf.MigrationsDir, time.Now())
	if err != nil {
		log.Fatal(err)
	}

	a, e := filepath.Abs(n)
	if e != nil {
		log.Fatal(e)
	}

	fmt.Println("goose: created", a)
}

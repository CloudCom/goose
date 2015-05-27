package main

import (
	"flag"
	"fmt"
	"os"
)

// shamelessly snagged from the go tool
// each command gets its own set of args,
// defines its own entry point, and provides its own help
type Command struct {
	Run  func(cmd *Command, args ...string)
	Flag flag.FlagSet

	Name  string
	Usage string

	Summary string
	Help    string
}

func (c *Command) Exec(args []string) {
	name := os.Args[0] + " " + c.Name
	c.Flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [args...] %s\n", name, c.Usage)
		c.Flag.PrintDefaults()
	}
	if err := c.Flag.Parse(args); err != nil {
		os.Exit(1)
	}
	c.Run(c, c.Flag.Args()...)
}

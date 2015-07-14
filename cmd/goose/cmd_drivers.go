package main

import "fmt"

var driversCmd = &Command{
	Name:    "drivers",
	Usage:   "",
	Summary: "Outputs the database drivers that the binary was created with",
	Help:    "",
	Run:     driversRun,
}

func driversRun(cmd *Command, args ...string) {
	fmt.Println("Drivers:")
	for _, d := range drivers {
		fmt.Printf("\t%s\n", d)
	}
}

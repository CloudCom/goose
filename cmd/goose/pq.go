// +build !nopq

package main

// including pq
import _ "github.com/lib/pq"

func init() {
	drivers = append(drivers, "pq")
}

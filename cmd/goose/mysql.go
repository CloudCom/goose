// +build !nomysql

package main

// including mysql
import _ "github.com/go-sql-driver/mysql"

func init() {
	drivers = append(drivers, "mysql")
}

// +build !nomymysql

package main

// including godrv
import _ "github.com/ziutek/mymysql/godrv"

func init() {
	drivers = append(drivers, "mymysql")
}

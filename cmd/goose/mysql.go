// +build !godrv,!pq,!sqlite

package main

// including mysql
import _ "github.com/go-sql-driver/mysql"

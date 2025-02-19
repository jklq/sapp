package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", os.Getenv("DATABASE_PATH"))
	if err != nil {
		panic(err)
	}

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}
	defer tx.Rollback()

	query, err := os.ReadFile("./schema.sql")
	if err != nil {
		panic(err)
	}

	_, err = tx.Exec(string(query))

	if err != nil {
		panic(err)
	}

	if err = tx.Commit(); err != nil {
		panic(fmt.Sprintf("Error committing transaction: %s\n", err))
	}
}

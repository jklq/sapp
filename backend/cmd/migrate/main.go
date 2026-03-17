package main

import (
	"database/sql"
	"log" // Use standard log for simplicity here
	"net/url"
	"os"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := os.Getenv("DATABASE_PATH")
	schemaPath := os.Getenv("SCHEMA_PATH")
	if dbPath == "" || schemaPath == "" {
		log.Fatal("DATABASE_PATH and SCHEMA_PATH environment variables must be set")
	}

	log.Printf("Attempting to open database: %s", dbPath)
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	log.Printf("Beginning transaction...")
	tx, err := db.Begin()
	if err != nil {
		log.Fatalf("Error beginning transaction: %v", err)
	}
	defer tx.Rollback() // Ensure rollback happens if commit fails or panics occur

	log.Printf("Reading schema file: %s", schemaPath)
	query, err := os.ReadFile(schemaPath)
	if err != nil {
		log.Fatalf("Error reading schema file: %v", err)
	}

	log.Printf("Executing schema SQL...")
	_, err = tx.Exec(string(query))
	if err != nil {
		// Rollback explicitly here before panicking to be clear
		tx.Rollback()
		log.Fatalf("Error executing schema SQL: %v", err)
	}
	log.Printf("Schema SQL executed successfully.")

	log.Printf("Committing transaction...")
	if err = tx.Commit(); err != nil {
		// Rollback already deferred, but log the commit error before panicking
		log.Fatalf("Error committing transaction: %v", err)
	}
	log.Printf("Transaction committed successfully. Migration complete.")
}

func sqliteDSN(path string) string {
	pragmas := url.Values{}
	pragmas.Add("_pragma", "busy_timeout(5000)")
	pragmas.Add("_pragma", "journal_mode(WAL)")
	pragmas.Add("_pragma", "synchronous(NORMAL)")
	pragmas.Add("_pragma", "foreign_keys(ON)")

	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}

	return path + separator + pragmas.Encode()
}

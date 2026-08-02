package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "dist/.shiori/shiori.db")
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("DELETE FROM media")
	if err != nil {
		log.Fatalf("delete media failed: %v", err)
	}
	_, err = db.Exec("DELETE FROM job_queue")
	if err != nil {
		log.Fatalf("delete job_queue failed: %v", err)
	}
	fmt.Println("Database wiped for testing.")
}

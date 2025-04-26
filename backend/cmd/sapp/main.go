package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"

	"git.sr.ht/~relay/sapp-backend/category"
	"git.sr.ht/~relay/sapp-backend/pay"
	"github.com/rs/cors"
	_ "modernc.org/sqlite"
)

func main() {
	mux := http.NewServeMux()

	db, err := sql.Open("sqlite", os.Getenv("DATABASE_PATH"))

	if err != nil {
		panic(err)
	}

	defer db.Close()

	mux.HandleFunc("POST /v1/pay/{shared_status}/{amount}/{category}", pay.HandlePayRoute(db))
	mux.HandleFunc("GET /v1/categories", category.HandleGetCategories(db))

	port := os.Getenv("PORT")

	handler := cors.Default().Handler(mux)

	err = http.ListenAndServe(fmt.Sprintf(":%v", port), handler)
	if err != nil {
		panic(err)
	}
}

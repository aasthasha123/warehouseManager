package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"coldstorage/internal/api"
	"coldstorage/internal/store"
)

//go:embed web
var webFS embed.FS

func main() {
	ctx := context.Background()

	// dsn := os.Getenv("DATABASE_URL")
	dsn := "postgresql://warehouse_db_rz8e_user:k7uN3b7PnNdDxGRThtcJshFyWb4nisMP@dpg-d9aun95aeets739vl67g-a.singapore-postgres.render.com/warehouse_db_rz8e"
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set — point it at your Postgres instance, " +
			"e.g. the connection string from Render's Postgres dashboard " +
			"(postgres://user:pass@host:5432/dbname?sslmode=require)")
	}

	db, err := store.Open(ctx, dsn)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer db.Close()

	// The embedded FS has "web/" as its root; strip that prefix so
	// index.html is served at "/" rather than "/web/".
	webRoot, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("failed to mount embedded frontend: %v", err)
	}

	server := api.NewServer(db, webRoot)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("coldstorage backend + frontend listening on %s (postgres connected)", addr)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatal(err)
	}
}

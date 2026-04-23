package main

import (
	"log"

	"github.com/nicolasticot/cass/server/internal/config"
	"github.com/nicolasticot/cass/server/internal/db"
	"github.com/nicolasticot/cass/server/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DataDir)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	if err := web.Run(cfg, database); err != nil {
		log.Fatalf("web: %v", err)
	}
}

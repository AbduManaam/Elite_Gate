package main

import (
	"log"

	"edgecore/internal/admin/app"
	"edgecore/internal/config"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("cannot load config: %v", err)
	}

	a, err := app.StartApp(cfg)
	if err != nil {
		log.Fatalf("cannot start admin app: %v", err)
	}

	if err := a.Server.Run(); err != nil {
		a.Logger.Fatal().Err(err).Msg("server exited with error")
	}
}

package main

import (
	"log"

	"elitegate/internal/config"
	"elitegate/internal/gateway/app"
)

func main() {
	//Load config 
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("cannot load config: %v", err)
	}

	// 2. Start app (logger + router + server)
	a, err := app.StartApp(cfg)
	if err != nil {
		log.Fatalf("cannot start app: %v", err)
	}

	// 3. Run (blocks until Ctrl+C / SIGTERM)
	if err := a.Server.Run(); err != nil {  //internal/gateway/server/server.go:
		a.Logger.Fatal().Err(err).Msg("server exited with error")
	}
}
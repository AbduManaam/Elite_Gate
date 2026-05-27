package main

import (
	"fmt"
	"os"

	"elitegate/internal/config"
	"elitegate/internal/gateway/app"
)

func main() {
	//Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Start app (logger + router + server)
	a, err := app.StartApp(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot start app: %v\n", err)
		os.Exit(1)
	}

	// 3. Run (blocks until Ctrl+C / SIGTERM)
	if err := a.Server.Run(); err != nil { //internal/gateway/server/server.go:
		a.Logger.Fatal().Err(err).Msg("server exited with error")
	}
}

package main

import (
	"fmt"
	"os"

	"elitegate/internal/admin/app"
	"elitegate/internal/config"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot load config: %v\n", err)
		os.Exit(1)
	}

	a, err := app.StartApp(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot start admin app: %v\n", err)
		os.Exit(1)
	}

	if err := a.Server.Run(); err != nil {
		a.Logger.Fatal().Err(err).Msg("server exited with error")
	}
}

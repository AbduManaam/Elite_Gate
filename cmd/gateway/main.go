// package main

// import (
// 	"context"
// 	"edgecore/internal/gateway/middleware"
// 	"edgecore/internal/gateway/proxy"
// 	"log"
// 	"net/http"
// 	"os"
// 	"os/signal"
// 	"syscall"
// 	"time"
// )

// func main() {
//     p, err := proxy.New("http://localhost:9090")
//     if err != nil {
//         log.Fatalf("failed to create proxy: %v", err)
//     }

//     mux := http.NewServeMux()

//     mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
//         w.Header().Set("Content-Type", "application/json")
//         w.Write([]byte(`{"status":"ok"}`))
//     })

//     mux.Handle("/api/", http.StripPrefix("/api", p))

//     handler := middleware.Chain(
// 		mux,
//         middleware.Recovery,
//         middleware.Logger,
//     )

// 	// custom HTTP server configuration
//     server := &http.Server{
//         Addr:         ":8080",
//         Handler:      handler,
//         ReadTimeout:  15 * time.Second,  //Maximum time allowed to READ incoming request.If request isn't fully received in 15s terminate connection
//         WriteTimeout: 30 * time.Second,  //Maximum time allowed to WRITE response back to client. 
//         IdleTimeout:  60 * time.Second,
//     }

//     go func() {
//         log.Println("Gateway running on :8080")
//         if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
//             log.Fatalf("Gateway failed: %v", err)
//         }
//     }()

// 	// ── Graceful Shutdown  [Listen for OS signals (Ctrl+C or SIGTERM from Docker/Kubernetes)]

//     quit := make(chan os.Signal, 1)
//     signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
//     <-quit

//     ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//     defer cancel()
//     server.Shutdown(ctx)
// }

package main

import (
	"log"

	"edgecore/internal/config"
	"edgecore/internal/gateway/app"
)

func main() {
	// ── 1. Load config ────────────────────────────────────────────────
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("cannot load config: %v", err)
	}

	// ── 2. Start app (logger + router + server) ───────────────────────
	a, err := app.StartApp(cfg)
	if err != nil {
		log.Fatalf("cannot start app: %v", err)
	}

	// ── 3. Run (blocks until Ctrl+C / SIGTERM) ────────────────────────
	if err := a.Server.Run(); err != nil {
		a.Logger.Fatal().Err(err).Msg("server exited with error")
	}
}
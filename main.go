package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type apiConfig struct {
	fileserverHits int
}

func main() {
	r := chi.NewRouter()
	// mux := http.NewServeMux()
	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir("./")))
	cfg := &apiConfig{}

	r.Handle("/app", cfg.middlewareMetricsInc(appHandler))
	r.Handle("/app/*", cfg.middlewareMetricsInc(appHandler))

	corsMux := middlewareCors(r)

	r.Mount("/api", apiRouter(r, cfg))
	r.Mount("/admin", adminRouter(r, cfg))

	server := &http.Server{
		Addr:    ":8080",
		Handler: corsMux,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Cannot start the server: %v", err)
	}
}

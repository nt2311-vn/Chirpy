package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type apiConfig struct {
	fileserverHits int
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits++
		next.ServeHTTP(w, r)
	})
}

func main() {
	r := chi.NewRouter()
	// mux := http.NewServeMux()
	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir("./")))
	cfg := &apiConfig{}

	r.Handle("/app", cfg.middlewareMetricsInc(appHandler))
	r.Handle("/app/*", cfg.middlewareMetricsInc(appHandler))

	corsMux := middlewareCors(r)

	/*

		r.Get("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		r.HandleFunc("/api/reset", func(w http.ResponseWriter, r *http.Request) {
			cfg.fileserverHits = 0
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Counter reset"))
		})

		r.Get("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits)))
		})
	*/

	r.Mount("/api", apiRouter(r, cfg))

	server := &http.Server{
		Addr:    ":8080",
		Handler: corsMux,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Cannot start the server: %v", err)
	}
}

func middlewareCors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func apiRouter(r *chi.Mux, cfg *apiConfig) http.Handler {
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Hits: %d", cfg.fileserverHits)))
	})

	r.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits = 0
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Counter reset"))
	})

	return r
}

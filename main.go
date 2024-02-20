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

func apiRouter(r *chi.Mux, cfg *apiConfig) http.Handler {
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	r.HandleFunc("/reset", func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits = 0
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Counter reset"))
	})

	return r
}

func adminRouter(r *chi.Mux, cfg *apiConfig) http.Handler {
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`
		<html>

		<body>
			<h1>Welcome, Chirpy Admin</h1>
			<p>Chirpy has been visited %d times!</p>
		</body>

		</html>

		`, cfg.fileserverHits)))
	})
	return r
}

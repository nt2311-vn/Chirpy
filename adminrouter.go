package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

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

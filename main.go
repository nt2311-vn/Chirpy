package main

import (
	"encoding/json"
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

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits++
		next.ServeHTTP(w, r)
	})
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

	r.Post("/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		type chirpreq struct {
			Reqmsg string `json:"body,omitempty"`
		}

		type chirpres struct {
			Resmsg bool `json:"valid,omitempty"`
		}

		type errorres struct {
			Errmsg string `json:"error,omitempty"`
		}
		decoder := json.NewDecoder(r.Body)
		params := chirpreq{}

		if err := decoder.Decode(&params); err != nil {
			log.Printf("Error decoding json body: %v", err)
			errormsg := errorres{
				Errmsg: fmt.Sprintf("%v", err),
			}

			resMsg, _ := json.Marshal(errormsg)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			w.Write(resMsg)
		}

		if len(params.Reqmsg) > 140 {
			errormsg := errorres{
				Errmsg: "Chirp is too long",
			}
			resMsg, _ := json.Marshal(errormsg)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			w.Write(resMsg)

		}

		okMsg := chirpres{Resmsg: true}
		encodeMsg, _ := json.Marshal(okMsg)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(encodeMsg)
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

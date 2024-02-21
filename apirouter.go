package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

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

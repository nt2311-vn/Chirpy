package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

func (cfg *apiConfig) handlerPolkaWebhooks(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID int `json:"user_id"`
		} `json:"data"`
	}

	authHeader := r.Header.Get("Authorization")

	if authHeader == "" {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized webhook")
		return
	}

	splitAuth := strings.Split(authHeader, " ")

	polka_token := os.Getenv("POLKA_KEY")

	if len(splitAuth) < 2 || splitAuth[1] != polka_token {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized webhook")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	if params.Event != "user.upgraded" {
		respondWithJSON(w, http.StatusOK, struct{}{})
		return
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't convert user ID")
		return
	}

	_, err = cfg.DB.UpgradedUser(params.Data.UserID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "User not exist")
		return
	}

	respondWithJSON(w, http.StatusOK, struct{}{})
}

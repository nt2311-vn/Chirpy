package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

type apiConfig struct {
	fileserverHits int
}

type DB struct {
	path string
	mux  *sync.RWMutex
}

type Chirp struct {
	Id   int    `json:"id"`
	Body string `json:"body,omitempty"`
}

type DBStructure struct {
	Chirp map[int]Chirp `json:"chirps"`
}

func main() {
	const rootPath = "."
	const port = "8080"

	apiConfg := apiConfig{fileserverHits: 0}
	router := chi.NewRouter()

	fsHandler := apiConfg.middlewareMetricsInc(
		http.StripPrefix("/app", http.FileServer(http.Dir(rootPath))),
	)

	router.Handle("/app", fsHandler)
	router.Handle("/app/*", fsHandler)

	apiRouter := chi.NewRouter()
	apiRouter.Get("/healthz", handlerReadiness)
	apiRouter.Get("/reset", apiConfg.handlerReset)
	apiRouter.Post("/validate_chirp", handlerChirpValidate)
	router.Mount("/api", apiRouter)

	adminRouter := chi.NewRouter()
	adminRouter.Get("/metrics", apiConfg.handlerMetrics)
	router.Mount("/admin", adminRouter)

	corsMux := middlewareCors(router)

	server := &http.Server{
		Addr:    ":" + port,
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

func handlerReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(http.StatusText(http.StatusOK)))
}

func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`
		<html>
		<body>
			<h1>Welcome, Chirpy Admin</h1>
			<p>Chirpy has been visited %dtimes!</p>
		</body>
		</html>
		`, cfg.fileserverHits)))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits++
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits = 0
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset to 0"))
}

func respondWithError(w http.ResponseWriter, statusCode int, msg string) {
	if statusCode > 499 {
		log.Printf("Responding with 5xx error: %s", msg)
	}

	type errorResponse struct {
		Error string `json:"error,omitempty"`
	}

	respondWithJSON(w, statusCode, errorResponse{Error: msg})
}

func respondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	dat, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}
	w.WriteHeader(statusCode)
	w.Write(dat)
}

func handlerChirpValidate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body,omitempty"`
	}

	type returnVals struct {
		CleanBody string `json:"cleaned_body,omitempty"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	if err := decoder.Decode(&params); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	const maxChirpLength = 140

	if len(params.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}

	respondWithJSON(w, http.StatusOK, returnVals{CleanBody: replaceProfane(params.Body)})
}

func replaceProfane(msg string) string {
	profaneWords := map[string]bool{"kerfuffle": true, "sharbert": true, "fornax": true}
	words := strings.Split(msg, " ")

	for index, word := range words {
		_, isProfance := profaneWords[strings.ToLower(word)]

		if isProfance {
			words[index] = "****"
		}

	}

	return strings.Join(words, " ")
}

func (db *DB) ensureDB() error {
	db.mux.Lock()
	defer db.mux.Unlock()

	if _, err := os.Stat(db.path); os.IsNotExist(err) {
		return db.writeDB(DBStructure{Chirp: make(map[int]Chirp)})
	}
	return nil
}

func (db *DB) writeDB(dbStructure DBStructure) error {
	db.mux.Lock()
	defer db.mux.Unlock()

	bytes, err := json.Marshal(dbStructure)
	if err != nil {
		return err
	}

	return os.WriteFile(db.path, bytes, 0644)
}

func (db *DB) loadDB() (DBStructure, error) {
	db.mux.Lock()
	defer db.mux.RUnlock()

	bytes, err := os.ReadFile(db.path)
	if err != nil {
		return DBStructure{}, err
	}

	var dbStructure DBStructure
	err = json.Unmarshal(bytes, &dbStructure)

	return dbStructure, err
}

func NewDB(path string) (*DB, error) {
	db := &DB{path: path, mux: new(sync.RWMutex)}

	initData := []Chirp{
		{Id: 1, Body: "This is the first chirp ever!"},
		{Id: 2, Body: "Hello, world!"},
	}

	data, err := json.Marshal(initData)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) CreateChirp(body string) (Chirp, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return Chirp{}, err
	}

	newId := len(dbStruct.Chirp) + 1

	chirp := Chirp{Id: newId, Body: body}
	dbStruct.Chirp[newId] = chirp

	err = db.writeDB(dbStruct)
	return chirp, err
}

func (db *DB) GetChirps() ([]Chirp, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return nil, err
	}

	chirps := make([]Chirp, 0, len(dbStruct.Chirp))

	for _, chirp := range chirps {
		chirps = append(chirps, chirp)
	}

	sort.Slice(chirps, func(i, j int) bool {
		return chirps[i].Id < chirps[j].Id
	})

	return chirps, nil
}

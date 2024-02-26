package main

import (
	"encoding/json"
	"errors"
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
	DB             *DB
}

type DB struct {
	path string
	mu   *sync.RWMutex
}

type Chirp struct {
	Id   int    `json:"id"`
	Body string `json:"body,omitempty"`
}

type DBStructure struct {
	Chirps map[int]Chirp `json:"chirps"`
}

func NewDB(path string) (*DB, error) {
	db := &DB{path: path, mu: &sync.RWMutex{}}
	err := db.ensureDB()

	return db, err
}

func (db *DB) CreateChirp(body string) (Chirp, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return Chirp{}, err
	}

	newId := len(dbStruct.Chirps) + 1

	chirp := Chirp{Id: newId, Body: body}
	dbStruct.Chirps[newId] = chirp

	err = db.writeDB(dbStruct)
	if err != nil {
		return Chirp{}, err
	}

	return chirp, nil
}

func (db *DB) GetChirps() ([]Chirp, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return nil, err
	}

	chirps := make([]Chirp, 0, len(dbStruct.Chirps))

	for _, chirp := range chirps {
		chirps = append(chirps, chirp)
	}

	return chirps, nil
}

func (db *DB) createDB() error {
	dbStruct := DBStructure{Chirps: map[int]Chirp{}}

	return db.writeDB(dbStruct)
}

func (db *DB) ensureDB() error {
	_, err := os.ReadFile(db.path)
	if errors.Is(err, os.ErrNotExist) {
		return db.createDB()
	}

	return err
}

func (db *DB) loadDB() (DBStructure, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	dbStruct := DBStructure{}

	dat, err := os.ReadFile(db.path)

	if errors.Is(err, os.ErrNotExist) {
		return dbStruct, err
	}

	err = json.Unmarshal(dat, &dbStruct)
	if err != nil {
		return dbStruct, err
	}

	return dbStruct, nil
}

func (db *DB) writeDB(dbStructure DBStructure) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	dat, err := json.Marshal(dbStructure)
	if err != nil {
		return err
	}

	err = os.WriteFile(db.path, dat, 0600)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	const rootPath = "."
	const port = "8080"

	db, err := NewDB("./database.json")
	if err != nil {
		log.Fatal(err)
	}

	apiConfg := apiConfig{fileserverHits: 0, DB: db}
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

func validateChirp(body string) (string, error) {
	const maxChirpLength = 140
	if len(body) > maxChirpLength {
		return "", errors.New("Chirp is too long")
	}

	cleaned := replaceProfane(body)

	return cleaned, nil
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

func (cfg *apiConfig) handlerChirpsCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body,omitempty"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	cleaned, err := validateChirp(params.Body)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	chirp, err := cfg.DB.CreateChirp(cleaned)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp")
		return
	}

	respondWithJSON(w, http.StatusCreated, Chirp{
		Id:   chirp.Id,
		Body: chirp.Body,
	})
}

func (cfg *apiConfig) handlerChirpsRetrieve(w http.ResponseWriter, r *http.Request) {
	dbChirps, err := cfg.DB.GetChirps()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve chierps")
	}

	chirps := []Chirp{}

	for _, dbChirp := range dbChirps {
		chirps = append(chirps, Chirp{Id: dbChirp.Id, Body: dbChirp.Body})
	}

	sort.Slice(chirps, func(i, j int) bool {
		return chirps[i].Id < chirps[j].Id
	})

	respondWithJSON(w, http.StatusOK, chirps)
}

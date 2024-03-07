package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

type apiConfig struct {
	fileserverHits int
	DB             *DB
	jwtSecret      string
}

type DB struct {
	path string
	mu   *sync.RWMutex
}

type Chirp struct {
	Body string `json:"body,omitempty"`
	Id   int    `json:"id"`
}

type User struct {
	Email    string `json:"email,omitempty"`
	Password string `json:"password,omitempty"`
	Id       int    `json:"id"`
}

type DBStructure struct {
	Chirps     map[int]Chirp         `json:"chirps"`
	Users      map[int]User          `json:"users"`
	Revocation map[string]Revocation `json:"revocation"`
}

type TokenType string

const (
	TokenTypeAccess  TokenType = "chirpy-access"
	TokenTypeRefresh TokenType = "chirpy-refresh"
)

type Revocation struct {
	Token     string    `json:"token"`
	RevokedAt time.Time `json:"revoked_at"`
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

	for _, chirp := range dbStruct.Chirps {
		chirps = append(chirps, chirp)
	}

	return chirps, nil
}

func (db *DB) GetChirpId(id int) (Chirp, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return Chirp{}, err
	}

	chirp, ok := dbStruct.Chirps[id]

	if !ok {
		return Chirp{}, os.ErrNotExist
	}

	return chirp, nil
}

func (db *DB) CreateUser(email, hashPass string) (User, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return User{}, err
	}

	user, err := db.GetUser(email)

	if err == nil {
		return user, nil
	}

	newId := len(dbStruct.Users) + 1
	user = User{
		Id:       newId,
		Email:    email,
		Password: string(hashPass),
	}
	dbStruct.Users[newId] = user

	err = db.writeDB(dbStruct)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func (db *DB) UpdateUser(id int, email, hashPass string) (User, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return User{}, err
	}

	user, ok := dbStruct.Users[id]

	if !ok {
		return User{}, os.ErrNotExist
	}

	user.Email = email
	user.Password = string(hashPass)

	dbStruct.Users[id] = user

	err = db.writeDB(dbStruct)
	if err != nil {
		return User{}, err
	}

	return user, nil
}

func (db *DB) GetUser(email string) (User, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return User{}, err
	}

	for _, user := range dbStruct.Users {
		if user.Email == email {
			return user, nil
		}
	}

	return User{}, os.ErrNotExist
}

func (db *DB) RevokeToken(token string) error {
	dbStruct, err := db.loadDB()
	if err != nil {
		return err
	}

	revocation := Revocation{
		Token:     token,
		RevokedAt: time.Now().UTC(),
	}

	dbStruct.Revocation[token] = revocation

	err = db.writeDB(dbStruct)
	if err != nil {
		return err
	}

	return nil
}

func (db *DB) IsTokenRevoked(token string) (bool, error) {
	dbStruct, err := db.loadDB()
	if err != nil {
		return false, err
	}

	revocation, ok := dbStruct.Revocation[token]

	if !ok {
		return false, nil
	}

	if revocation.RevokedAt.IsZero() {
		return false, nil
	}

	return true, nil
}

func (db *DB) createDB() error {
	dbStruct := DBStructure{
		Chirps: map[int]Chirp{},
		Users:  map[int]User{},
	}

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
	godotenv.Load()

	jwtSecret := os.Getenv("JWT_SECRET")

	db, err := NewDB("./database.json")
	if err != nil {
		log.Fatal(err)
	}

	apiConfg := apiConfig{fileserverHits: 0, DB: db, jwtSecret: jwtSecret}
	router := chi.NewRouter()

	fsHandler := apiConfg.middlewareMetricsInc(
		http.StripPrefix("/app", http.FileServer(http.Dir(rootPath))),
	)

	router.Handle("/app", fsHandler)
	router.Handle("/app/*", fsHandler)

	apiRouter := chi.NewRouter()
	apiRouter.Get("/healthz", handlerReadiness)
	apiRouter.Get("/reset", apiConfg.handlerReset)

	apiRouter.Post("/chirps", apiConfg.handlerChirpsCreate)
	apiRouter.Get("/chirps", apiConfg.handlerChirpsRetrieve)
	apiRouter.Get("/chirps/{chirpId}", apiConfg.handlerChirpGet)

	apiRouter.Post("/users", apiConfg.handlerUserCreate)
	apiRouter.Put("/users", apiConfg.handlerUpdateUser)

	apiRouter.Post("/login", apiConfg.handlerUserLogin)
	apiRouter.Post("/revoke", apiConfg.hanlderRevoke)
	apiRouter.Post("/refresh", apiConfg.handlerRefresh)

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
	} else {
		fmt.Println("chirpy is running on port " + port)
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

func HashPassword(password string) (string, error) {
	hashPass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashPass), nil
}

func CheckPassword(password, hashStr string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashStr), []byte(password))
}

func MakeJWT(
	userID int,
	tokenSecret string,
	expiresIn time.Duration,
	tokenType TokenType,
) (string, error) {
	signingKey := []byte(tokenSecret)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Issuer:    string(tokenType),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
		Subject:   fmt.Sprintf("%d", userID),
	})

	return token.SignedString(signingKey)
}

func ValidateJWT(tokenString, tokenSecret string) (string, error) {
	claimStruct := &jwt.RegisteredClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claimStruct,
		func(token *jwt.Token) (interface{}, error) { return []byte(tokenSecret), nil },
	)
	if err != nil {
		return "", err
	}

	if !token.Valid {
		return "", errors.New("Invalid token")
	}

	return claimStruct.Subject, nil
}

func RefreshToken(tokenString, tokenSecret string) (string, error) {
	claimStruct := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claimStruct,
		func(t *jwt.Token) (interface{}, error) { return []byte(tokenSecret), nil },
	)
	if err != nil {
		return "", err
	}

	if !token.Valid {
	}

	userID, _ := strconv.Atoi(claimStruct.Subject)

	newToken, err := MakeJWT(userID, tokenSecret, time.Hour, TokenTypeAccess)
	if err != nil {
		return "", err
	}

	return newToken, nil
}

func GetBearerToken(headers http.Header) (string, error) {
	authHeader := headers.Get("Authorization")

	if authHeader == "" {
		return "", errors.New("No Authorization header")
	}

	splitAuth := strings.Split(authHeader, " ")

	if len(splitAuth) < 2 || splitAuth[0] != "Bearer" {
		return "", errors.New("Invalid Authorization header")
	}

	return splitAuth[1], nil
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

func (cfg *apiConfig) handlerChirpGet(w http.ResponseWriter, r *http.Request) {
	chirpIdStr := chi.URLParam(r, "chirpId")

	chirpID, err := strconv.Atoi(chirpIdStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid chrip ID")
		return
	}

	chirp, err := cfg.DB.GetChirpId(chirpID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondWithError(w, http.StatusNotFound, "Chirp not found")
		} else {
			respondWithError(w, http.StatusInternalServerError, "Error retrieving chirp")
		}
		return
	}

	respondWithJSON(w, http.StatusOK, chirp)
}

func (cfg *apiConfig) handlerUserCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode the post request")
		return
	}

	hashedPass, err := HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash password")
		return
	}

	user, err := cfg.DB.CreateUser(params.Email, hashedPass)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Coudn't create user")
		return
	}

	respondWithJSON(w, http.StatusCreated, User{Id: user.Id, Email: user.Email})
}

func (cfg *apiConfig) handlerUserLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
		Expires  int    `json:"expires_in_seconds,omitempty"`
	}

	type response struct {
		User
		Token        string `json:"token"`
		RefreshToken string `json:"refresh_token"`
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(
			w,
			http.StatusInternalServerError,
			"Couldn't decode the post login request",
		)
		return
	}

	user, err := cfg.DB.GetUser(params.Email)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "User not found")
		return
	}

	err = CheckPassword(params.Password, user.Password)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid password")
	}

	acessToken, err := MakeJWT(user.Id, cfg.jwtSecret, time.Hour, TokenTypeAccess)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't create access token")
	}

	refreshToken, err := MakeJWT(user.Id, cfg.jwtSecret, time.Hour*30*24, TokenTypeRefresh)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Could not create refresh token")
	}

	respondWithJSON(
		w,
		http.StatusOK,
		response{
			User:         User{Id: user.Id, Email: user.Email},
			Token:        acessToken,
			RefreshToken: refreshToken,
		},
	)
}

func (cfg *apiConfig) handlerUpdateUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	type response struct {
		User
	}

	token, err := GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT")
		return
	}

	subject, err := ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Could not validate JWT")
		return
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}

	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode the parameter")
		return
	}

	hashedPass, err := HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash password")
		return
	}

	userIDInt, err := strconv.Atoi(subject)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't convert user ID")
		return
	}

	user, err := cfg.DB.UpdateUser(userIDInt, params.Email, hashedPass)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update user")
		return
	}

	respondWithJSON(w, http.StatusOK, response{
		User: User{Id: user.Id, Email: user.Email},
	})
}

func (cfg *apiConfig) handlerRefresh(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Token string `json:"token"`
	}

	refreshToken, err := GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find JWT")
	}

	isRevoked, err := cfg.DB.IsTokenRevoked(refreshToken)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't check if token was revoked")
		return
	}

	if isRevoked {
		respondWithError(w, http.StatusUnauthorized, "Token has been revoked")
		return
	}

	accessToken, err := RefreshToken(refreshToken, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT")
		return
	}

	respondWithJSON(w, http.StatusOK, response{Token: accessToken})
}

func (cfg *apiConfig) hanlderRevoke(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't find JWT")
		return
	}

	err = cfg.DB.RevokeToken(refreshToken)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't revoke session")
		return
	}

	respondWithJSON(w, http.StatusOK, struct{}{})
}

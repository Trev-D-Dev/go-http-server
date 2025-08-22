package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Trev-D-Dev/go-http-server/internal/auth"
	"github.com/Trev-D-Dev/go-http-server/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
	secret         string
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, err := w.Write([]byte("OK"))
	if err != nil {
		fmt.Printf("error occurred writing body: %v\n", err)
		return
	}
}

func (cfg *apiConfig) numRequestsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)

	htmlString := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())

	_, err := w.Write([]byte(htmlString))
	if err != nil {
		fmt.Printf("error occured writing body: %v\n", err)
		return
	}
}

func (cfg *apiConfig) resetRequests(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.Header().Set("Content-Type", "application/json")
		errMsg := "403 Forbidden"
		http.Error(w, errMsg, 403)
		return
	}

	cfg.fileserverHits = atomic.Int32{}

	cfg.db.ResetUsers(r.Context())

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	_, err := w.Write([]byte("Hits Reset"))
	if err != nil {
		fmt.Printf("error occured writing body: %v\n", err)
		return
	}
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		errMsg := fmt.Sprintf("Error decoding params: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	type User struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}

	hashPass, err := auth.HashPassword(params.Password)
	if err != nil {
		errMsg := fmt.Sprintf("Error hashing password: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashPass,
	})
	if err != nil {
		errMsg := fmt.Sprintf("Error creating user: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	userJson := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}

	dat, err := json.Marshal(userJson)
	if err != nil {
		errMsg := fmt.Sprintf("Error marsalling json: %v", err)
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {
	bannedWords := [6]string{"kerfuffle", "sharbert", "fornax", "Kerfuffle", "Sharbert", "Fornax"}

	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		errMsg := fmt.Sprintf("Error decoding params: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errMsg := fmt.Sprintf("Error retrieving token: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	uid, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		errMsg := "unauthorized to post chirp"
		http.Error(w, errMsg, http.StatusUnauthorized)
		return
	}

	chirpLen := len(params.Body)

	if chirpLen == 0 {
		http.Error(w, "invalid chirp", 400)
		return
	} else if chirpLen > 140 {
		http.Error(w, "chirp is too long", 400)
		return
	}

	for i := range bannedWords {
		params.Body = strings.Replace(params.Body, bannedWords[i], "****", -1)
	}

	chirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   params.Body,
		UserID: uid,
	})
	if err != nil {
		errMsg := fmt.Sprintf("Error creating chirp: %v\n", err)
		http.Error(w, errMsg, 400)
		return
	}

	chirpJson := Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}

	dat, err := json.Marshal(chirpJson)
	if err != nil {
		errMsg := fmt.Sprintf("Error marshalling json: %v\n", err)
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func (cfg *apiConfig) allChirpsHandler(w http.ResponseWriter, r *http.Request) {
	dbChirps, err := cfg.db.GetChirps(r.Context())
	if err != nil {
		errMsg := fmt.Sprintf("Error retrieving chirps: %v\n", err)
		http.Error(w, errMsg, 400)
		return
	}

	chirps := []Chirp{}
	for _, dbChirp := range dbChirps {
		chirps = append(chirps, Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		})
	}

	dat, err := json.Marshal(chirps)
	if err != nil {
		errMsg := fmt.Sprintf("Error marshalling JSON: %v\n", err)
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) singleChirpHandler(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")

	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		errMsg := fmt.Sprintf("Error parsing UUID: %v\n", err)
		http.Error(w, errMsg, 400)
		return
	}

	dbChirp, err := cfg.db.GetChirp(r.Context(), chirpUUID)
	if err != nil {
		http.Error(w, "404 Chirp Not Found", 404)
		return
	}

	chirp := Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	}

	dat, err := json.Marshal(chirp)
	if err != nil {
		errMsg := fmt.Sprintf("Error marshalling JSON: %v\n", err)
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		errMsg := fmt.Sprintf("Error decoding params: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	user, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		http.Error(w, "error: incorrect email", http.StatusUnauthorized)
		return
	}

	err = auth.CheckPasswordHash(user.HashedPassword, params.Password)
	if err != nil {
		http.Error(w, "error: incorrect password", http.StatusUnauthorized)
		return
	}

	type User struct {
		ID           uuid.UUID `json:"id"`
		CreatedAt    time.Time `json:"created_at"`
		UpdatedAt    time.Time `json:"updated_at"`
		Email        string    `json:"email"`
		Token        string    `json:"token"`
		RefreshToken string    `json:"refresh_token"`
	}

	duration := time.Hour

	token, err := auth.MakeJWT(user.ID, cfg.secret, duration)
	if err != nil {
		errMsg := fmt.Sprintf("error creating token: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	hexToken, err := auth.MakeRefreshToken()
	if err != nil {
		errMsg := fmt.Sprintf("Error creating hexToken: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	expiresAt := time.Now().UTC().Add(60 * 24 * time.Hour)

	refreshToken, err := cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     hexToken,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		errMsg := fmt.Sprintf("Error creating refresh token: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	userJson := User{
		ID:           user.ID,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
		Email:        user.Email,
		Token:        token,
		RefreshToken: refreshToken,
	}

	dat, err := json.Marshal(userJson)
	if err != nil {
		errMsg := fmt.Sprintf("Error marshalling json: %v", err)
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errMsg := fmt.Sprintf("error retrieving token: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	refToken, err := cfg.db.GetRefreshByToken(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	} else if refToken.RevokedAt.Valid {
		http.Error(w, "refresh token revoked", http.StatusUnauthorized)
		return
	} else if time.Now().UTC().After(refToken.ExpiresAt) {
		http.Error(w, "refresh token expired", http.StatusUnauthorized)
		return
	}

	newToken, err := auth.MakeJWT(refToken.UserID, cfg.secret, time.Hour)
	if err != nil {
		errMsg := fmt.Sprintf("error creating new token: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	type tokenReturn struct {
		Token string `json:"token"`
	}

	returnJson := tokenReturn{
		Token: newToken,
	}

	dat, err := json.Marshal(returnJson)
	if err != nil {
		errMsg := fmt.Sprintf("Error marshalling json: %v", err)
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) handleRevoke(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errMsg := fmt.Sprintf("error retrieving token: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	err = cfg.db.RevokeRefreshToken(r.Context(), token)
	if err != nil {
		errMsg := fmt.Sprintf("error revoking token: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(204)
}

func (cfg *apiConfig) passEmailChangeHandler(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		errMsg := fmt.Sprintf("Error decoding params: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	hashPass, err := auth.HashPassword(params.Password)
	if err != nil {
		errMsg := fmt.Sprintf("error hashing password: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errMsg := fmt.Sprintf("error retrieving token: %v", err)
		http.Error(w, errMsg, http.StatusUnauthorized)
		return
	}

	currentUserID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		errMsg := fmt.Sprintf("error validating jwt: %v", err)
		http.Error(w, errMsg, http.StatusUnauthorized)
		return
	}

	updatedUser, err := cfg.db.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:             currentUserID,
		Email:          params.Email,
		HashedPassword: hashPass,
	})
	if err != nil {
		errMsg := fmt.Sprintf("error updating user: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	type User struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}

	returnUser := User{
		ID:        updatedUser.ID,
		CreatedAt: updatedUser.CreatedAt,
		UpdatedAt: updatedUser.UpdatedAt,
		Email:     updatedUser.Email,
	}

	dat, err := json.Marshal(returnUser)
	if err != nil {
		errMsg := fmt.Sprintf("Error marshalling json: %v", err)
		http.Error(w, errMsg, 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) deleteChirp(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")

	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		errMsg := fmt.Sprintf("error parsing chirpID: %v", err)
		http.Error(w, errMsg, 400)
		return
	}

	dbChirp, err := cfg.db.GetChirp(r.Context(), chirpUUID)
	if err != nil {
		http.Error(w, "404 Chirp Not Found", 404)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		errMsg := fmt.Sprintf("error retrieving token: %v", err)
		http.Error(w, errMsg, http.StatusUnauthorized)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		errMsg := fmt.Sprintf("error validating token: %v", err)
		http.Error(w, errMsg, http.StatusUnauthorized)
		return
	}

	if dbChirp.UserID != userID {
		http.Error(w, "403 Forbidden", http.StatusForbidden)
		return
	}

	err = cfg.db.DeleteChirp(r.Context(), database.DeleteChirpParams{
		ID:     dbChirp.ID,
		UserID: userID,
	})
	if err != nil {
		http.Error(w, "Chirp deletion unsuccessful", 400)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(204)
}

func main() {

	godotenv.Load()
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", os.Getenv("DB_URL"))
	if err != nil {
		fmt.Printf("error occured: %v\n", err)
		os.Exit(1)
	}

	dbQueries := database.New(db)

	apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
		db:             dbQueries,
		platform:       platform,
		secret:         os.Getenv("SECRET"),
	}

	handler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))

	sMux := http.NewServeMux()
	sMux.Handle("/app/", apiCfg.middlewareMetricsInc(handler))
	sMux.Handle("/assets/", http.FileServer(http.Dir(".")))
	sMux.HandleFunc("GET /api/healthz", readinessHandler)
	sMux.HandleFunc("GET /admin/metrics", apiCfg.numRequestsHandler)
	sMux.HandleFunc("POST /admin/reset", apiCfg.resetRequests)
	//sMux.HandleFunc("POST /api/validate_chirp", apiCfg.validateChirp)
	sMux.HandleFunc("POST /api/users", apiCfg.createUser)
	sMux.HandleFunc("POST /api/chirps", apiCfg.chirpsHandler)
	sMux.HandleFunc("GET /api/chirps", apiCfg.allChirpsHandler)
	sMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.singleChirpHandler)
	sMux.HandleFunc("POST /api/login", apiCfg.loginHandler)
	sMux.HandleFunc("POST /api/refresh", apiCfg.refreshHandler)
	sMux.HandleFunc("POST /api/revoke", apiCfg.handleRevoke)
	sMux.HandleFunc("PUT /api/users", apiCfg.passEmailChangeHandler)
	sMux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.deleteChirp)

	server := http.Server{
		Addr:    ":8080",
		Handler: sMux,
	}

	err = server.ListenAndServe()

	if err != nil {
		fmt.Printf("error occured: %v\n", err)
		os.Exit(1)
	}
}

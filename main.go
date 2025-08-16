package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Trev-D-Dev/go-http-server/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	db             *database.Queries
	platform       string
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
		errorHandler(w, errMsg)
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

func errorHandler(w http.ResponseWriter, errMsg string) {
	type errorResponse struct {
		Error string `json:"error"`
	}

	errResp := errorResponse{
		Error: errMsg,
	}

	dat, err := json.Marshal(errResp)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(500)
		return
	}

	if errMsg == "403 Forbidden" {
		w.WriteHeader(403)
		w.Write(dat)
		return
	} else if errMsg == "404 Chirp Not Found" {
		w.WriteHeader(404)
		w.Write(dat)
		return
	}

	w.WriteHeader(400)
	w.Write(dat)
}

/*
func (cfg *apiConfig) validateChirp(w http.ResponseWriter, r *http.Request) {
	bannedWords := [6]string{"kerfuffle", "sharbert", "fornax", "Kerfuffle", "Sharbert", "Fornax"}

	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		errMsg := fmt.Sprintf("Error decoding params: %v", err)
		errorHandler(w, errMsg)
		return
	} else {
		type returnVal struct {
			CleanedBody string `json:"cleaned_body"`
		}

		if len(params.Body) <= 140 && len(params.Body) != 0 {

			for i := range bannedWords {
				if strings.Contains(params.Body, bannedWords[i]) {
					params.Body = strings.Replace(params.Body, bannedWords[i], "****", -1)
				}
			}

			retJson := returnVal{
				CleanedBody: params.Body,
			}

			dat, err := json.Marshal(retJson)
			if err != nil {
				log.Printf("Error marshalling JSON: %s", err)
				w.WriteHeader(500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write(dat)
		} else if len(params.Body) == 0 {
			errorHandler(w, "Invalid chirp")
			return
		} else {
			errorHandler(w, "Chirp is too long")
			return
		}
	}
}*/

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		errMsg := fmt.Sprintf("Error decoding params: %v", err)
		errorHandler(w, errMsg)
		return
	}

	type User struct {
		ID        uuid.UUID `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Email     string    `json:"email"`
	}

	user, err := cfg.db.CreateUser(r.Context(), params.Email)
	if err != nil {
		errMsg := fmt.Sprintf("Error creating user: %v", err)
		errorHandler(w, errMsg)
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
		errorHandler(w, errMsg)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(201)
	w.Write(dat)
}

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {
	bannedWords := [6]string{"kerfuffle", "sharbert", "fornax", "Kerfuffle", "Sharbert", "Fornax"}

	type parameters struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		errMsg := fmt.Sprintf("Error decoding params: %v", err)
		errorHandler(w, errMsg)
		return
	}

	chirpLen := len(params.Body)

	if chirpLen == 0 {
		errorHandler(w, "Invalid chirp")
		return
	} else if chirpLen > 140 {
		errorHandler(w, "Chirp is too long")
		return
	}

	for i := range bannedWords {
		if strings.Contains(params.Body, bannedWords[i]) {
			params.Body = strings.Replace(params.Body, bannedWords[i], "****", -1)
		}
	}

	chirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   params.Body,
		UserID: params.UserID,
	})
	if err != nil {
		errMsg := fmt.Sprintf("Error creating chirp: %v\n", err)
		errorHandler(w, errMsg)
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
		errorHandler(w, errMsg)
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
		errorHandler(w, errMsg)
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
		errorHandler(w, errMsg)
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
		errorHandler(w, errMsg)
		return
	}

	dbChirp, err := cfg.db.GetChirp(r.Context(), chirpUUID)
	if err != nil {
		errorHandler(w, "404 Chirp Not Found")
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
		errorHandler(w, errMsg)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.Write(dat)
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {

}

func main() {

	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	platform := os.Getenv("PLATFORM")

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		fmt.Printf("error occured: %v\n", err)
		os.Exit(1)
	}

	dbQueries := database.New(db)

	apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
		db:             dbQueries,
		platform:       platform,
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

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
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
	cfg.fileserverHits = atomic.Int32{}

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
	w.WriteHeader(400)
	w.Write(dat)
}

func (cfg *apiConfig) validateChirp(w http.ResponseWriter, r *http.Request) {
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
			Valid bool `json:"valid"`
		}

		if len(params.Body) <= 140 && len(params.Body) != 0 {
			retJson := returnVal{
				Valid: true,
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
}

func main() {

	apiCfg := apiConfig{
		fileserverHits: atomic.Int32{},
	}

	handler := http.StripPrefix("/app/", http.FileServer(http.Dir(".")))

	sMux := http.NewServeMux()
	sMux.Handle("/app/", apiCfg.middlewareMetricsInc(handler))
	sMux.Handle("/assets/", http.FileServer(http.Dir(".")))
	sMux.HandleFunc("GET /api/healthz", readinessHandler)
	sMux.HandleFunc("GET /admin/metrics", apiCfg.numRequestsHandler)
	sMux.HandleFunc("POST /admin/reset", apiCfg.resetRequests)
	sMux.HandleFunc("POST /api/validate_chirp", apiCfg.validateChirp)

	server := http.Server{
		Addr:    ":8080",
		Handler: sMux,
	}

	err := server.ListenAndServe()

	if err != nil {
		fmt.Printf("error occured: %v\n", err)
		os.Exit(1)
	}
}

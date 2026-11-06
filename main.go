package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/lucascherwenka/chirpy/internal/database"
)

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	plat := os.Getenv("PLATFORM")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return
	}
	dbQueries := database.New(db)
	newMux := http.NewServeMux()
	newServer := &http.Server{
		Addr:    ":8080",
		Handler: newMux,
	}
	apiCfg := apiConfig{database: dbQueries, platform: plat}
	newMux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	newMux.HandleFunc("GET /api/healthz", readinessEndpoint)
	newMux.HandleFunc("GET /admin/metrics", apiCfg.reportHandler)
	newMux.HandleFunc("POST /admin/reset", apiCfg.reportReset)
	newMux.HandleFunc("POST /api/users", apiCfg.createUser)
	newMux.HandleFunc("POST /api/chirps", apiCfg.createChirp)
	newMux.HandleFunc("GET /api/chirps", apiCfg.getChirps)
	newMux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.getChirp)
	fmt.Println("Now serving webpage on :8080")
	newServer.ListenAndServe()
}

func readinessEndpoint(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) reportReset(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(403)
		w.Write([]byte("Forbidden"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	err := cfg.database.DeleteUsers(r.Context())
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	cfg.fileserverHits.Store(0)
	w.WriteHeader(200)
	w.Write([]byte("Reset"))
}

func (cfg *apiConfig) reportHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	resp := fmt.Sprintf(`<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>`, cfg.fileserverHits.Load())
	w.WriteHeader(200)
	w.Write([]byte(resp))
}

type apiConfig struct {
	fileserverHits atomic.Int32
	database       *database.Queries
	platform       string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	w.Write(response)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	respondWithJSON(w, code, map[string]string{"error": msg})
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) createChirp(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	type requestBody struct {
		Body   string    `json:"body"`
		UserID uuid.UUID `json:"user_id"`
	}

	type cleanedBody struct {
		Cleaned_body string `json:"cleaned_body"`
	}

	type responseBody struct {
		Valid bool `json:"valid"`
	}

	dat, err := io.ReadAll(r.Body)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}
	params := requestBody{}
	err = json.Unmarshal(dat, &params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}

	words := strings.Split(params.Body, " ")

	if len(params.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	} else {
		for i, word := range words {
			if strings.ToLower(word) == "kerfuffle" || strings.ToLower(word) == "sharbert" || strings.ToLower(word) == "fornax" {
				words[i] = "****"
			}
		}
	}
	cleaned := cleanedBody{
		Cleaned_body: strings.Join(words, " "),
	}
	chirp, err := cfg.database.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleaned.Cleaned_body,
		UserID: params.UserID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Something went wrong")
		return
	}
	respondWithJSON(w, 201, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	})

}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	type incEmail struct {
		Email string `json:"email"`
	}

	params := incEmail{}

	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, err := cfg.database.CreateUser(r.Context(), params.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	newUser := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}
	respondWithJSON(w, 201, newUser)
}

func (cfg *apiConfig) getChirps(w http.ResponseWriter, r *http.Request) {
	list, err := cfg.database.GetChirps(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	myList := []Chirp{}
	for _, item := range list {
		myList = append(myList, Chirp{
			ID:        item.ID,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
			Body:      item.Body,
			UserID:    item.UserID,
		})
	}
	respondWithJSON(w, 200, myList)
}

func (cfg *apiConfig) getChirp(w http.ResponseWriter, r *http.Request) {
	val := r.PathValue("chirpID")
	valId, err := uuid.Parse(val)
	if err != nil {
		respondWithError(w, 400, "Failed to parse UUID")
		return
	}
	chirp, err := cfg.database.GetChirp(r.Context(), valId)
	if err != nil {
		respondWithError(w, 404, "Not Found")
		return
	}
	validChirp := Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserID:    chirp.UserID,
	}
	respondWithJSON(w, 200, validChirp)
}

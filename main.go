package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/go-chi/render"
	"github.com/gorilla/mux"
)

type URL struct {
	ShortURL string `json:"ShortUrl"`
	Code     int    `json:"Code"`
}

var db *bolt.DB
var dbMutex sync.Mutex
var baseURL string
var maxLongURLLength int

func main() {
	// 读取环境变量
	baseURL = getEnv("SHORT_URL_BASE", "http://short.url")
	maxLongURLLength = getEnvAsInt("MAX_LONG_URL_LENGTH", 100000)
	dbPath := getEnv("DB_PATH", "./short_url_db")
	retentionDuration := getEnvAsInt("URL_RETENTION_DURATION", 604800)

	var err error
	db, err = bolt.Open(dbPath, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 设置路由
	r := mux.NewRouter()
	r.HandleFunc("/short", shortenHandler).Methods("POST")
	r.HandleFunc("/{shortID}", redirectHandler).Methods("GET")

	// 定期清理过期短链接
	go func() {
		ticker := time.NewTicker(time.Duration(retentionDuration) * time.Second)
		for {
			<-ticker.C
			cleanUpExpiredLinks()
		}
	}()

	log.Printf("Server started at :8000")
	http.ListenAndServe(":8000", r)
}

func shortenHandler(w http.ResponseWriter, r *http.Request) {
	longURL := r.FormValue("longUrl")
	if longURL == "" {
		render.JSON(w, r, map[string]string{"error": "Missing long_url parameter"})
		return
	}

	decodedURL, err := base64.StdEncoding.DecodeString(longURL)
	if err != nil || len(decodedURL) > maxLongURLLength {
		render.JSON(w, r, map[string]string{"error": "Invalid or too long URL"})
		return
	}

	shortID := generateShortLink()
	shortURL := fmt.Sprintf("%s/%s", baseURL, shortID)

	dbMutex.Lock()
	err = db.Update(func(tx *bolt.Tx) error {
		b, _ := tx.CreateBucketIfNotExists([]byte("urls"))
		return b.Put([]byte(shortID), decodedURL)
	})
	dbMutex.Unlock()
	if err != nil {
		render.JSON(w, r, map[string]string{"error": "Failed to store URL"})
		return
	}

	resp := URL{
		ShortURL: shortURL,
		Code:     1,
	}
	render.JSON(w, r, resp)
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	shortID := vars["shortID"]

	var longURL []byte
	dbMutex.Lock()
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("urls"))
		if b == nil {
			return fmt.Errorf("Bucket not found")
		}
		longURL = b.Get([]byte(shortID))
		return nil
	})
	dbMutex.Unlock()
	if err != nil || longURL == nil {
		http.NotFound(w, r)
		return
	}

	http.Redirect(w, r, string(longURL), http.StatusFound)
}

const letterBytes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// generate is a function that takes an integer bits and returns a string.
// The function generates a random string of length equal to bits using the letterBytes slice.
// The letterBytes slice contains characters that can be used to generate a random string.
// The generation of the random string is based on the current time using the UnixNano() function.
func GenerateRandomString(bits int) string {
	// Create a byte slice b of length bits.
	b := make([]byte, bits)

	// Create a new random number generator with the current time as the seed.
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Generate a random byte for each element in the byte slice b using the letterBytes slice.
	for i := range b {
		b[i] = letterBytes[r.Intn(len(letterBytes))]
	}

	// Convert the byte slice to a string and return it.
	return string(b)
}

func generateShortLink() string {
	return GenerateRandomString(8)
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if valueStr, exists := os.LookupEnv(key); exists {
		value, err := strconv.Atoi(valueStr)
		if err == nil {
			return value
		}
	}
	return defaultValue
}

func cleanUpExpiredLinks() {
	// 清理过期短链接逻辑
}


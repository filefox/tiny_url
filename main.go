package main

import (
        "encoding/base64"
        "encoding/json"
        "fmt"
        "log"
        "math/rand"
        "net/http"
        "net/url"
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

type UserInfo struct {
        Username string `json:"username"`
        Password string `json:"password"`
        LongURL  string `json:"long_url"`
}

var db *bolt.DB
var dbMutex sync.Mutex
var baseURL string
var baseURLParsed *url.URL
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

        // 解析基础 URL
        baseURLParsed, err = url.Parse(baseURL)
        if err != nil {
                log.Fatal("Invalid base URL:", err)
        }

        // 设置路由
        r := mux.NewRouter()
        r.HandleFunc("/short", shortenHandler).Methods("POST")
        r.HandleFunc("/", redirectHandler).Methods("GET")
        r.HandleFunc("/update", updateHandler).Methods("PUT")

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

        username := shortID              // 用户名使用 4 位 shortID
        password := generateRandomString(2) // 密码长度为 2

        dbMutex.Lock()
        err = db.Update(func(tx *bolt.Tx) error {
                b, _ := tx.CreateBucketIfNotExists([]byte("urls"))
                userInfo := UserInfo{
                        Username: username,
                        Password: password,
                        LongURL:  string(decodedURL),
                }
                userInfoBytes, err := json.Marshal(userInfo)
                if err != nil {
                        return err
                }
                return b.Put([]byte(shortID), userInfoBytes)
        })
        dbMutex.Unlock()
        if err != nil {
                render.JSON(w, r, map[string]string{"error": "Failed to store URL"})
                return
        }

        baseURLParsed.User = url.UserPassword(username, password)
        shortURL := baseURLParsed.String()

        resp := URL{
                ShortURL: shortURL,
                Code:     1,
        }
        render.JSON(w, r, resp)
}

func redirectHandler(w http.ResponseWriter, r *http.Request) {
        username, password, ok := r.BasicAuth()
        if !ok {
                w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
        }

        var userInfo UserInfo
        dbMutex.Lock()
        err := db.View(func(tx *bolt.Tx) error {
                b := tx.Bucket([]byte("urls"))
                if b == nil {
                        return fmt.Errorf("Bucket not found")
                }
                c := b.Cursor()
                for k, v := c.First(); k != nil; k, v = c.Next() {
                        var tempUserInfo UserInfo
                        err := json.Unmarshal(v, &tempUserInfo)
                        if err != nil {
                                return err
                        }
                        if tempUserInfo.Username == username {
                                userInfo = tempUserInfo
                                return nil
                        }
                }
                return fmt.Errorf("User info not found")
        })
        dbMutex.Unlock()

        if err != nil {
                http.NotFound(w, r)
                return
        }

        if password != userInfo.Password {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
        }

        http.Redirect(w, r, userInfo.LongURL, http.StatusFound)
}

func updateHandler(w http.ResponseWriter, r *http.Request) {
        username, password, ok := r.BasicAuth()
        if !ok {
                w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
        }

        newLongURL := r.FormValue("longUrl")
        if newLongURL == "" {
                render.JSON(w, r, map[string]string{"error": "Missing long_url parameter"})
                return
        }

        dbMutex.Lock()
        err := db.Update(func(tx *bolt.Tx) error {
                b := tx.Bucket([]byte("urls"))
                if b == nil {
                        return fmt.Errorf("Bucket not found")
                }
                c := b.Cursor()
                for k, v := c.First(); k != nil; k, v = c.Next() {
                        var tempUserInfo UserInfo
                        err := json.Unmarshal(v, &tempUserInfo)
                        if err != nil {
                                return err
                        }
                        if tempUserInfo.Username == username && tempUserInfo.Password == password {
                                tempUserInfo.LongURL = newLongURL
                                updatedUserInfoBytes, err := json.Marshal(tempUserInfo)
                                if err != nil {
                                        return err
                                }
                                return b.Put(k, updatedUserInfoBytes)
                        }
                }
                return fmt.Errorf("User info not found")
        })
        dbMutex.Unlock()

        if err != nil {
                render.JSON(w, r, map[string]string{"error": "Failed to update URL"})
                return
        }

        render.JSON(w, r, map[string]string{"message": "URL updated successfully"})
}

const letterBytes = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func generateRandomString(bits int) string {
        b := make([]byte, bits)
        r := rand.New(rand.NewSource(time.Now().UnixNano()))
        for i := range b {
                b[i] = letterBytes[r.Intn(len(letterBytes))]
        }
        return string(b)
}

func generateShortLink() string {
        return generateRandomString(4) // 生成 4 位 shortID
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

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/golang-migrate/migrate/v4"
	cassandramigrate "github.com/golang-migrate/migrate/v4/database/cassandra"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/redis/go-redis/v9"
)

var session *gocql.Session
var rdb *redis.Client
var base62Hashed = shuffleChars(os.Getenv("BASE62_SECRET"))

func main() {
	cluster := gocql.NewCluster(getCassandrasHost()...)
	cluster.Consistency = gocql.Quorum

	var err error
	session, err = cluster.CreateSession()
	if err != nil {
		log.Fatal("cassandra connect:", err)
	}

	ensureKeyspace(session)

	cluster.Keyspace = "myshortlinks"
	session, err = cluster.CreateSession()
	if err != nil {
		log.Fatal("cassandra reconnect:", err)
	}
	defer session.Close()

	runMigrations(session)
	connectRedis()

	http.HandleFunc("GET /health", handleHealth)
	http.HandleFunc("POST /link", handleCreateShortLink)
	http.HandleFunc("GET /{url}", handleRedirect)

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func connectRedis() {
	rdb = redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    os.Getenv("REDIS_MASTER_NAME"),
		SentinelAddrs: strings.Split(os.Getenv("REDIS_SENTINELS"), ","),
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal("redis connect:", err)
	}
	rdb.SetNX(ctx, "short_link_counter", 10_000_000, 0)
	log.Println("Redis ready")
}

func ensureKeyspace(s *gocql.Session) {
	err := s.Query(`CREATE KEYSPACE IF NOT EXISTS myshortlinks
		WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}`).Exec()
	if err != nil {
		log.Fatal("create keyspace:", err)
	}
	log.Println("Keyspace ready")
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleCreateShortLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	shortCode, err := generateCode(r.Context())
	if err != nil {
		http.Error(w, "failed to generate code", http.StatusInternalServerError)
		return
	}

	err = session.Query(
		`INSERT INTO links (short_code, original_url, created_at) VALUES (?, ?, ?)`,
		shortCode, body.URL, time.Now(),
	).Exec()
	if err != nil {
		http.Error(w, "failed to save", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"data": shortCode})
}

func handleRedirect(w http.ResponseWriter, r *http.Request) {
    code := r.PathValue("url")
    ctx := r.Context()

    originalURL, err := rdb.Get(ctx, code).Result()
    if err == nil {
        http.Redirect(w, r, originalURL, http.StatusFound)
        return
    }

    err = session.Query(
        `SELECT original_url FROM links WHERE short_code = ?`, code,
    ).Scan(&originalURL)

    if err == gocql.ErrNotFound {
        http.Error(w, "link not found", http.StatusNotFound)
        return
    }
    if err != nil {
        http.Error(w, "unknown error", http.StatusInternalServerError)
        return
    }
	
    rdb.Set(ctx, code, originalURL, 24*time.Hour)
    http.Redirect(w, r, originalURL, http.StatusFound)
}

func runMigrations(s *gocql.Session) {
	driver, err := cassandramigrate.WithInstance(s, &cassandramigrate.Config{
		KeyspaceName: "myshortlinks",
	})
	if err != nil {
		log.Fatal("migrate driver:", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "cassandra", driver)
	if err != nil {
		log.Fatal("migrate init:", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal("migrate up:", err)
	}

	log.Println("Migrations applied successfully")
}

func generateCode(ctx context.Context) (string, error) {
	id, err := rdb.Incr(ctx, "short_link_counter").Result()
	if err != nil {
		return "", err
	}
	return toBase62(id, base62Hashed), nil
}

func getCassandrasHost() []string {
	hosts := os.Getenv("CASSANDRA_HOSTS")
	if hosts == "" {
		return []string{"cassandra-1"}
	}
	return strings.Split(hosts, ",")
}

func shuffleChars(secret string) string {
	chars := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	hash := sha256.Sum256([]byte(secret))
	seed := int64(hash[0]) | int64(hash[1])<<8 | int64(hash[2])<<16 | int64(hash[3])<<24
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(chars), func(i, j int) {
		chars[i], chars[j] = chars[j], chars[i]
	})
	return string(chars)
}

func toBase62(n int64, chars string) string {
	result := ""
	for n > 0 {
		result = string(chars[n%62]) + result
		n /= 62
	}
	return result
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"github.com/golang-migrate/migrate/v4"
	cassandramigrate "github.com/golang-migrate/migrate/v4/database/cassandra"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

var session *gocql.Session

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

	http.HandleFunc("GET /health", handleHealth)
	http.HandleFunc("POST /link", handleCreateShortLink)
	http.HandleFunc("GET /{url}", handleRedirect)

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
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

	shortCode := "asgasgasgasgasgasgasg"

	err := session.Query(
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

	var originalURL string
	err := session.Query(
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

func getCassandrasHost() []string {
	hosts := os.Getenv("CASSANDRA_HOSTS")
	if hosts == "" {
		return []string{"cassandra-1"}
	}
	return strings.Split(hosts, ",")
}

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"github.com/gocql/gocql"
	"github.com/golang-migrate/v4"
	cassandramigrate "github.com/golang-migrate/migrate/v4/database/cassandra"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	cluster := gocql.NewCluster("cassandra-1","cassandra-2","cassandra-3","cassandra-4")
	cluster.Consistency = gocql.Quorum

	session, err := cluster.CreateSession()

	if err != nil {
		log.Fatal("cassandra connect:", err)
	}

	defer session.Close()

	runMigrations(session)

	http.HandleFunc("GET /health", handleHealth)
	http.HandleFunc("GET /shorten", handleShorten)

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleShorten(w http.ResponseWriter, r *http.Request){
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "shorten endpoint"})
}

func runMigrations(session *gocql.Session) {
	driver, err := cassandramigrate.WithInstance(session, &cassandramigrate.Config{
		KeyspaceName: "myshortlinks",
	})
	if err != nil {
		log.Fatal("migrate driver: ", err)
	}
	
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "cassandra", driver)
	if err != nil {
		log.Fatal("migrate init:", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal("migrate up", err)
	}

	log.Println("Migrations applied successfully")
}
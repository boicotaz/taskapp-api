package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
)

var db *sql.DB

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT title, description, status FROM tasks")
	if err != nil {
		http.Error(w, "Failed to query tasks", http.StatusInternalServerError)
		log.Println("Query error:", err)
		return
	}
	defer rows.Close()

	tasks := []map[string]interface{}{}
	for rows.Next() {
		var title, description, status string
		if err := rows.Scan(&title, &description, &status); err != nil {
			http.Error(w, "Failed to scan task", http.StatusInternalServerError)
			log.Println("Scan error:", err)
			return
		}
		tasks = append(tasks, map[string]interface{}{
			"title":       title,
			"description": description,
			"status":      status,
		})
	}

	if err := rows.Err(); err != nil {
		http.Error(w, "Error iterating rows", http.StatusInternalServerError)
		log.Println("Rows iteration error:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tasks); err != nil {
		http.Error(w, "Failed to encode tasks", http.StatusInternalServerError)
		log.Println("JSON encoding error:", err)
	}
}

// Readiness: checks DB connectivity.
func readinessHandler(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		http.Error(w, "db not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

func main() {

	// DB connection values from env (K8s ConfigMap/Secret friendly)
	dbHost := os.Getenv("DB_HOST")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbPort := os.Getenv("DB_PORT")

	// --- Open DB and verify with Ping ---
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPassword, dbName, "disable")

	var db_err error
	db, db_err = sql.Open("postgres", dsn)
	if db_err != nil {
		log.Fatal("sql.Open error:", db_err)
	}
	if db_err := db.Ping(); db_err != nil {
		log.Fatal("DB ping failed:", db_err)
	}
	log.Println("✅ Connected to Postgres")

	http.HandleFunc("/ready", readinessHandler)

	http.HandleFunc("/tasks", tasksHandler)

	// Read PORT from environment variable, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s...\n", port)
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal(err)
	}
}

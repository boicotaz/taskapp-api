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

// Task represents a row in the tasks table
type Task struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetTasks(w, r)
	case http.MethodPut:
		handleUpdateTask(w, r)
	case http.MethodPost:
		handleCreateTask(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetTasks(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, title, description, status FROM tasks")
	if err != nil {
		http.Error(w, "failed to query tasks", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Status); err != nil {
			http.Error(w, "failed to scan task", http.StatusInternalServerError)
			return
		}
		tasks = append(tasks, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	var t Task

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if t.ID == 0 {
		http.Error(w, "task id is required", http.StatusBadRequest)
		return
	}

	result, err := db.Exec(`
		UPDATE tasks
		SET title = $1,
		    description = $2,
		    status = $3
		WHERE id = $4
	`, t.Title, t.Description, t.Status, t.ID)

	if err != nil {
		http.Error(w, "Failed to update task", http.StatusInternalServerError)
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("task updated"))
}


func handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var t Task

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Basic validation
	if t.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	// Optional default
	if t.Status == "" {
		t.Status = "todo"
	}

	err := db.QueryRow(`
		INSERT INTO tasks (title, description, status)
		VALUES ($1, $2, $3)
		RETURNING id
	`, t.Title, t.Description, t.Status).Scan(&t.ID)

	if err != nil {
		http.Error(w, "Failed to create task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(t)
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

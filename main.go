package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

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

type TaskEvent struct {
	ID        int       `json:"id,omitempty"`
	Action    string    `json:"action,omitempty"`
	Timestamp time.Time `json:"ts"`
	Level     string    `json:"level"`
}

func tasksHandler(taskQueue chan Task, logQueue chan TaskEvent) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodGet:
            handleGetTasks(w, r)
        case http.MethodPut:
            handleUpdateTask(w, r, logQueue)
        case http.MethodPost:
            handleCreateTask(w, r, taskQueue, logQueue)
        default:
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        }
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

func handleUpdateTask(w http.ResponseWriter, r *http.Request, logQueue chan TaskEvent) {
	var t Task

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if t.ID == 0 {
		logQueue <- TaskEvent{
			Action:    "update_failed_no_id",
			Level:     "error",
			Timestamp: time.Now(),
		}
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

	logQueue <- TaskEvent{
		ID:        t.ID,
		Action:    "updated",
		Level:     "info",
		Timestamp: time.Now(),
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("task updated"))
}


func handleCreateTask(w http.ResponseWriter, r *http.Request, taskQueue chan Task, logQueue chan TaskEvent) {
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

	// 👇 async work
	taskQueue <- t

	// Log the creation event
	logQueue <- TaskEvent{
		ID:        t.ID,
		Action:    "created",
		Level:     "info",
		Timestamp: time.Now(),
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

func taskWorker(taskQueue chan Task, logQueue chan TaskEvent) {
	startup_message := TaskEvent{
		Action:    "task_worker_started",
		Level:     "info",
		Timestamp: time.Now(),
	}
	logQueue <- startup_message

	for task := range taskQueue {
		// mark as processing
		logQueue <- TaskEvent{
			ID:    task.ID,
			Action:    "processing_started",
			Level:     "info",
			Timestamp: time.Now(),
		}

		if _, err := db.Exec("UPDATE tasks SET status = $1 WHERE id = $2", "processing", task.ID); err != nil {
			logQueue <- TaskEvent{
				ID:    task.ID,
				Action:    "processing_failed",
				Level:     "error",
				Timestamp: time.Now(),
			}
			continue
		}

		time.Sleep(5 * time.Second) // Simulate work

		if _, err := db.Exec("UPDATE tasks SET status = $1 WHERE id = $2", "done", task.ID); err != nil {
			logQueue <- TaskEvent{
				ID:    task.ID,
				Action:    "completion_failed",
				Level:     "error",
				Timestamp: time.Now(),
			}
			continue
		}

		logQueue <- TaskEvent{
			ID:    task.ID,
			Action:    "completed",
			Level:     "info",
			Timestamp: time.Now(),
		}

	}
}

func logWorker(logQueue chan TaskEvent) {

	startup_message, _ := json.Marshal(TaskEvent{
		Action:    "log_worker_started",
		Level:     "info",
		Timestamp: time.Now(),
	})
	log.Println(string(startup_message))

	for event := range logQueue {
		//log.Printf("Task %d %s at %s\n", event.TaskID, event.Action, event.Timestamp.Format(time.RFC3339))
		message, err := json.Marshal(event)
		if err != nil {
			log.Println("Failed to marshal log event:", err)
			continue
		}
		log.Println(string(message))
	}
}

func main() {
	// Disable timestamp in default logger for cleaner JSON logs
	log.SetFlags(0)

	// --- Start Log Worker ---
	var logQueue chan TaskEvent
	logQueue = make(chan TaskEvent, 100)

	go logWorker(logQueue)
	
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
	logQueue <- TaskEvent{
		Action:    "db_connected",
		Level:     "info",
		Timestamp: time.Now(),
	}

	// --- Start Task Worker ---
	var taskQueue chan Task
	taskQueue = make(chan Task, 100)

	go taskWorker(taskQueue, logQueue)

	http.HandleFunc("/ready", readinessHandler)

	http.HandleFunc("/tasks", tasksHandler(taskQueue, logQueue))

	// Read PORT from environment variable, default to 8080
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logQueue <- TaskEvent{
		Action:    "server_starting",
		Level:     "info",
		Timestamp: time.Now(),
	}
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatal(err)
	}
}

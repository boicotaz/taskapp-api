package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"taskapp/backend/internal/db"
	"taskapp/backend/internal/handler"
	"taskapp/backend/internal/model"
	"taskapp/backend/internal/worker"
)

func main() {
	log.SetFlags(0)

	logQueue := make(chan model.TaskEvent, 100)
	go worker.LogWorker(logQueue)

	database, err := db.Connect()
	if err != nil {
		msg, _ := json.Marshal(model.TaskEvent{Action: "db_connection_failed", Level: "fatal", Timestamp: time.Now()})
		log.Fatal(string(msg))
	}
	logQueue <- model.TaskEvent{Action: "db_connected", Level: "info", Timestamp: time.Now()}

	taskQueue := make(chan model.Task, 100)
	go worker.TaskWorker(taskQueue, logQueue)

	tasks := handler.NewTaskHandler(database, taskQueue, logQueue)
	http.Handle("/tasks", tasks)
	http.HandleFunc("/ready", tasks.Readiness)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logQueue <- model.TaskEvent{Action: "server_starting", Level: "info", Timestamp: time.Now()}

	srv := &http.Server{Addr: ":" + port}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			msg, _ := json.Marshal(model.TaskEvent{Action: "server_listen_failed", Level: "fatal", Timestamp: time.Now()})
			log.Fatal(string(msg))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logQueue <- model.TaskEvent{Action: "server_shutting_down", Level: "info", Timestamp: time.Now()}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		msg, _ := json.Marshal(model.TaskEvent{Action: "shutdown_timeout", Level: "fatal", Timestamp: time.Now()})
		log.Fatal(string(msg))
	}

	logQueue <- model.TaskEvent{Action: "server_stopped", Level: "info", Timestamp: time.Now()}
}

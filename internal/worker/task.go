package worker

import (
	"database/sql"
	"time"

	"taskapp/backend/internal/model"
)

func TaskWorker(db *sql.DB, taskQueue chan model.Task, logQueue chan model.TaskEvent) {
	logQueue <- model.TaskEvent{Action: "task_worker_started", Level: "info", Timestamp: time.Now()}

	for task := range taskQueue {
		logQueue <- model.TaskEvent{ID: task.ID, Action: "processing_started", Level: "info", Timestamp: time.Now()}

		if _, err := db.Exec("UPDATE tasks SET status = $1 WHERE id = $2", "processing", task.ID); err != nil {
			logQueue <- model.TaskEvent{ID: task.ID, Action: "processing_failed", Level: "error", Timestamp: time.Now()}
			continue
		}

		time.Sleep(5 * time.Second)

		if _, err := db.Exec("UPDATE tasks SET status = $1 WHERE id = $2", "done", task.ID); err != nil {
			logQueue <- model.TaskEvent{ID: task.ID, Action: "completion_failed", Level: "error", Timestamp: time.Now()}
			continue
		}

		logQueue <- model.TaskEvent{ID: task.ID, Action: "completed", Level: "info", Timestamp: time.Now()}
	}
}

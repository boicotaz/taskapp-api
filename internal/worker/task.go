package worker

import (
	"time"

	"taskapp/backend/internal/model"
)

// TaskWorker will be replaced with an SQS polling loop.
func TaskWorker(taskQueue chan model.Task, logQueue chan model.TaskEvent) {
	for task := range taskQueue {
		logQueue <- model.TaskEvent{ID: task.ID, Action: "task_received", Level: "info", Timestamp: time.Now()}
	}
}

package worker

import (
	"encoding/json"
	"log"
	"time"

	"taskapp/backend/internal/model"
)

func LogWorker(logQueue chan model.TaskEvent) {
	msg, _ := json.Marshal(model.TaskEvent{Action: "log_worker_started", Level: "info", Timestamp: time.Now()})
	log.Println(string(msg))

	for event := range logQueue {
		message, err := json.Marshal(event)
		if err != nil {
			log.Println("failed to marshal log event:", err)
			continue
		}
		log.Println(string(message))
	}
}

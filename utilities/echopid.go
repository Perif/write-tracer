package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// logPIDWithTimestamp logs the current process ID (PID) and a timestamp
// every specified interval.
func logPIDWithTimestamp(intervalSeconds int) {
	// Use a channel to listen for OS signals for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM) // Listen for Ctrl+C and termination signals

    // Get the current process ID
	pid := os.Getpid()
	log.Printf("Starting PID logger with PID %d. Logging every %d seconds...", pid, intervalSeconds)

	ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
	defer ticker.Stop() // Ensure the ticker is stopped when the function exits

	for {
		select {
		case <-ticker.C:

			// Get the current timestamp
			timestamp := time.Now().Format("2006-01-02 15:04:05") // Go's reference time format

			// Log the PID and timestamp
			log.Printf("PID: %d, Timestamp: %s", pid, timestamp)
		case <-stop:
			// Received a stop signal, gracefully exit
			log.Println("PID logger stopped by user.")
			return
		}
	}
}

func main() {
	// You can run the program with the default interval (5 seconds)
	logPIDWithTimestamp(5)

	// Or you can specify a different interval, for example, 2 seconds:
	// logPIDWithTimestamp(2)
}

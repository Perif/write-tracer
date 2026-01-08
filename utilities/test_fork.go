package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "child" {
		fmt.Printf("[Child  %d] Writing to stdout\n", os.Getpid())
		time.Sleep(2 * time.Second)
		fmt.Printf("[Child  %d] Done\n", os.Getpid())
		return
	}

	fmt.Printf("[Parent %d] Starting child process...\n", os.Getpid())

	cmd := exec.Command(os.Args[0], "child")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("[Parent %d] Failed to start child: %v\n", os.Getpid(), err)
		os.Exit(1)
	}

	fmt.Printf("[Parent %d] Child started with PID %d\n", os.Getpid(), cmd.Process.Pid)

	for i := 0; i < 5; i++ {
		fmt.Printf("[Parent %d] Loop %d\n", os.Getpid(), i)
		time.Sleep(1 * time.Second)
	}

	if err := cmd.Wait(); err != nil {
		fmt.Printf("[Parent %d] Child finished with error: %v\n", os.Getpid(), err)
	} else {
		fmt.Printf("[Parent %d] Child finished successfully\n", os.Getpid())
	}
}

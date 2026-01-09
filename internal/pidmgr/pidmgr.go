// Package pidmgr provides thread-safe management of tracked process IDs.
// It maintains a registry of parent PIDs and their threads, updating the eBPF
// tracked_pids map and automatically cleaning up when processes terminate.
package pidmgr

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/cilium/ebpf"
)

// TrackedProcess holds information about a registered parent process.
type TrackedProcess struct {
	ParentPID    uint32
	ThreadIDs    []uint32
	RegisteredAt time.Time
}

// PIDRegistry manages the set of tracked parent PIDs and their threads.
type PIDRegistry struct {
	mu            sync.RWMutex
	trackedPids   map[uint32]*TrackedProcess // parent PID -> process info
	ebpfMap       *ebpf.Map                  // tracked_pids eBPF map
	checkInterval time.Duration
}

// New creates a new PIDRegistry with the given eBPF tracked_pids map.
// checkInterval controls how often process liveness is checked (default 5s).
func New(ebpfMap *ebpf.Map, checkInterval time.Duration) *PIDRegistry {
	if checkInterval == 0 {
		checkInterval = 5 * time.Second
	}
	return &PIDRegistry{
		trackedPids:   make(map[uint32]*TrackedProcess),
		ebpfMap:       ebpfMap,
		checkInterval: checkInterval,
	}
}

// RegisterPID adds a parent PID and all its threads to the tracking registry.
// Returns the number of threads found, or an error if the process doesn't exist.
func (r *PIDRegistry) RegisterPID(pid uint32) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already registered
	if _, exists := r.trackedPids[pid]; exists {
		return 0, fmt.Errorf("PID %d is already registered", pid)
	}

	// Read threads from /proc
	tids, err := r.readThreads(pid)
	if err != nil {
		return 0, fmt.Errorf("failed to read threads for PID %d: %w", pid, err)
	}

	// Add all threads to eBPF map
	val := uint32(1)
	for _, tid := range tids {
		if err := r.ebpfMap.Update(tid, val, ebpf.UpdateAny); err != nil {
			// Rollback on error
			for _, t := range tids {
				_ = r.ebpfMap.Delete(t)
			}
			return 0, fmt.Errorf("failed to update eBPF map for TID %d: %w", tid, err)
		}
	}

	r.trackedPids[pid] = &TrackedProcess{
		ParentPID:    pid,
		ThreadIDs:    tids,
		RegisteredAt: time.Now(),
	}

	slog.Info("Registered PID for tracking", "pid", pid, "threads", len(tids))
	return len(tids), nil
}

// UnregisterPID removes a parent PID and all its threads from tracking.
func (r *PIDRegistry) UnregisterPID(pid uint32) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	proc, exists := r.trackedPids[pid]
	if !exists {
		return fmt.Errorf("PID %d is not registered", pid)
	}

	// Remove all threads from eBPF map
	for _, tid := range proc.ThreadIDs {
		if err := r.ebpfMap.Delete(tid); err != nil {
			slog.Warn("Failed to delete TID from eBPF map", "tid", tid, "error", err)
		}
	}

	delete(r.trackedPids, pid)
	slog.Info("Unregistered PID from tracking", "pid", pid)
	return nil
}

// List returns a copy of all currently tracked processes.
func (r *PIDRegistry) List() []TrackedProcess {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]TrackedProcess, 0, len(r.trackedPids))
	for _, proc := range r.trackedPids {
		result = append(result, *proc)
	}
	return result
}

// IsRegistered checks if a PID is currently registered.
func (r *PIDRegistry) IsRegistered(pid uint32) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.trackedPids[pid]
	return exists
}

// StartLivenessMonitor starts a goroutine that periodically checks if tracked
// processes are still alive. Dead processes are automatically unregistered.
// The monitor stops when the context is cancelled.
func (r *PIDRegistry) StartLivenessMonitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(r.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.checkLiveness()
			}
		}
	}()
}

// checkLiveness removes any tracked PIDs whose processes have terminated.
func (r *PIDRegistry) checkLiveness() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for pid, proc := range r.trackedPids {
		if !r.processExists(pid) {
			// Remove threads from eBPF map
			for _, tid := range proc.ThreadIDs {
				_ = r.ebpfMap.Delete(tid)
			}
			delete(r.trackedPids, pid)
			slog.Info("Auto-removed terminated process", "pid", pid)
		}
	}
}

// processExists checks if a process with the given PID exists.
func (r *PIDRegistry) processExists(pid uint32) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// readThreads returns all thread IDs for a given parent PID.
func (r *PIDRegistry) readThreads(pid uint32) ([]uint32, error) {
	entries, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	if err != nil {
		return nil, err
	}

	tids := make([]uint32, 0, len(entries))
	for _, entry := range entries {
		tid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		tids = append(tids, uint32(tid))
	}
	return tids, nil
}

// RefreshThreads updates the thread list for a tracked PID.
// This can be used to pick up newly spawned threads.
func (r *PIDRegistry) RefreshThreads(pid uint32) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	proc, exists := r.trackedPids[pid]
	if !exists {
		return 0, fmt.Errorf("PID %d is not registered", pid)
	}

	// Read current threads
	currentTids, err := r.readThreads(pid)
	if err != nil {
		return 0, err
	}

	// Build set of existing TIDs
	existingSet := make(map[uint32]bool)
	for _, tid := range proc.ThreadIDs {
		existingSet[tid] = true
	}

	// Add new threads to eBPF map
	val := uint32(1)
	newCount := 0
	for _, tid := range currentTids {
		if !existingSet[tid] {
			if err := r.ebpfMap.Update(tid, val, ebpf.UpdateAny); err != nil {
				slog.Warn("Failed to add new TID to eBPF map", "tid", tid, "error", err)
				continue
			}
			newCount++
		}
	}

	proc.ThreadIDs = currentTids
	return newCount, nil
}

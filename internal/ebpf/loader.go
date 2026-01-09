package ebpf

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"write-tracer/internal/config"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags $BPF_CFLAGS bpf ../../bpf/write_tracer.bpf.c -- -I../../bpf/headers

func Load(cfg config.Config) (*ebpf.Collection, []link.Link, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, nil, fmt.Errorf("remove memlock: %w", err)
	}

	spec, err := loadBpf()
	if err != nil {
		return nil, nil, fmt.Errorf("load spec: %w", err)
	}

	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return nil, nil, fmt.Errorf("create collection: %w", err)
	}

	bpfCfg := bpfConfig{
		TargetPid: cfg.TargetPID,
		NumFds:    cfg.NumFDs,
		TargetFds: cfg.TargetFDs,
	}
	if err := coll.Maps["config_map"].Update(uint32(0), bpfCfg, ebpf.UpdateAny); err != nil {
		coll.Close()
		return nil, nil, fmt.Errorf("update config map: %w", err)
	}

	count := 0
	// Only initialize from CLI PID if it's set
	if cfg.TargetPID != 0 {
		var err error
		count, err = InitTrackedPids(coll, cfg.TargetPID)
		if err != nil {
			coll.Close()
			return nil, nil, err
		}
		slog.Info("Initialized tracking", "target_pid", cfg.TargetPID, "threads_found", count)
	}

	links, err := attachTracepoints(coll)
	if err != nil {
		coll.Close()
		return nil, nil, err
	}

	return coll, links, nil
}

func InitTrackedPids(coll *ebpf.Collection, targetPID uint32) (int, error) {
	tids, err := os.ReadDir(fmt.Sprintf("/proc/%d/task", targetPID))
	if err != nil {
		return 0, fmt.Errorf("read threads: %w", err)
	}

	val := uint32(1)
	count := 0
	for _, entry := range tids {
		tid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		if err := coll.Maps["tracked_pids"].Update(uint32(tid), val, ebpf.UpdateAny); err != nil {
			return 0, fmt.Errorf("update tracked_pids for TID %d: %w", tid, err)
		}
		count++
	}
	return count, nil
}

func attachTracepoints(coll *ebpf.Collection) ([]link.Link, error) {
	lWrite, err := link.Tracepoint("syscalls", "sys_enter_write", coll.Programs["trace_write_enter"], nil)
	if err != nil {
		return nil, fmt.Errorf("attach write tracepoint: %w", err)
	}

	lFork, err := link.AttachRawTracepoint(link.RawTracepointOptions{
		Name:    "sched_process_fork",
		Program: coll.Programs["trace_sched_process_fork"],
	})
	if err != nil {
		lWrite.Close()
		return nil, fmt.Errorf("attach fork tracepoint: %w", err)
	}

	lExit, err := link.AttachRawTracepoint(link.RawTracepointOptions{
		Name:    "sched_process_exit",
		Program: coll.Programs["trace_sched_process_exit"],
	})
	if err != nil {
		lWrite.Close()
		lFork.Close()
		return nil, fmt.Errorf("attach exit tracepoint: %w", err)
	}

	return []link.Link{lWrite, lFork, lExit}, nil
}

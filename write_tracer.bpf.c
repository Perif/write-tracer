// write_tracer.bpf.c
//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

#define MAX_FDS 64

// Configuration structure
struct config {
    __u32 target_pid;
    __u32 num_fds;
    __u32 target_fds[MAX_FDS];
};

// Event structure
struct write_event {
    __u32 pid;
    __u32 tid;
    __u32 fd;
    __u64 count;
    __u64 timestamp;
    char comm[16];
};

// Maps
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct config);
} config_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

// Helper function to check if fd is in target list
static __always_inline int is_target_fd(struct config *cfg, __u32 fd) {
    for (int i = 0; i < MAX_FDS; i++) {
        if (i >= cfg->num_fds) break;
        if (cfg->target_fds[i] == fd) {
            return 1;
        }
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_write")
int trace_write_enter(struct trace_event_raw_sys_enter* ctx) {
    __u64 pid_tgid = bpf_get_current_pid_tgid();
    __u32 pid = pid_tgid >> 32;
    __u32 tid = (__u32)pid_tgid;
    
    // Get configuration
    __u32 key = 0;
    struct config *cfg = bpf_map_lookup_elem(&config_map, &key);
    if (!cfg) {
        return 0;
    }
    
    // Check if this is our target process
    if (cfg->target_pid != 0 && pid != cfg->target_pid) {
        return 0;
    }
    
    // Extract syscall arguments
    __u32 fd = (__u32)BPF_CORE_READ(ctx, args[0]);
    __u64 count = (__u64)BPF_CORE_READ(ctx, args[2]);
    
    // Check if this fd is in our target list
    if (cfg->num_fds > 0 && !is_target_fd(cfg, fd)) {
        return 0;
    }
    
    // Reserve space in ring buffer
    struct write_event *event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event) {
        return 0;
    }
    
    // Fill event data
    event->pid = pid;
    event->tid = tid;
    event->fd = fd;
    event->count = count;
    event->timestamp = bpf_ktime_get_ns();
    bpf_get_current_comm(&event->comm, sizeof(event->comm));
    
    // Submit event
    bpf_ringbuf_submit(event, 0);
    
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
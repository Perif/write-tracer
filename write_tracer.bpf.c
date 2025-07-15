// write_tracer.bpf.c
//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

#define MAX_FDS 64 // maximum number of file descriptors
#define MAX_DATA_SIZE 256 // max bytes retrieved from the write buffer
#define MAX_EXEC_NAME_SIZE 16 // max size of the program name

// Configuration structure
struct config {
    __u32 target_pid;
    __u32 num_fds;
    __u32 target_fds[MAX_FDS];
};

// Event structure, shared by the user space code
struct write_event {
    __u64 timestamp;
    __u64 count;
    __u32 pid;
    __u32 tid;
    __u32 fd;
    __u32 _padding; // Explicit padding for 8-byte alignment
    __u8 comm[MAX_EXEC_NAME_SIZE];
    __u8 data[MAX_DATA_SIZE];
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
    
    // The arguments to the write syscall are in the `ctx->args` array.
    // args[0] is the file descriptor (fd).
    // args[1] is the user-space buffer (buf).
    // args[2] is the count of bytes to write.
    __u64 fd = ctx->args[0];
    const char *buf = (const char*)ctx->args[1];
    __u64 count = ctx->args[2];
    
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
    event->pid = pid; // process ID
    event->tid = tid; // thread ID
    event->fd = fd; // file descriptor
    event->count = count; // get the number of elements 
    // get the time when the call is interpreted by epbf
    event->timestamp = bpf_ktime_get_ns(); 
    // get the current name of the process
    bpf_get_current_comm(event->comm, sizeof(event->comm));

    // Read the data from the user-space buffer.
    __u32 data_size = count < MAX_DATA_SIZE ? count : MAX_DATA_SIZE;
    bpf_probe_read_user(event->data, data_size, buf);

    
// #ifdef DEBUG
    // Logs can be seen with:
    // sudo cat /sys/kernel/debug/tracing/trace_pipe

    bpf_printk("trace_write_enter pid %d tid %d fd %d count %llu name %s data %s\n", 
        event->pid, event->tid, event->fd, event->count, (char*) event->comm, (char*) event->data);
// #endif
    // Submit event
    bpf_ringbuf_submit(event, 0);
    
    return 0;
}

char LICENSE[] SEC("license") = "GPL";
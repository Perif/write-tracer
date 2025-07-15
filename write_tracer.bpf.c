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
    u32 target_pid;
    u32 num_fds;
    u32 target_fds[MAX_FDS];
};

// Event structure
struct write_event {
    u32 pid;
    u32 tid;
    u32 fd;
    u64 count;
    u64 timestamp;
    u8 comm[MAX_EXEC_NAME_SIZE];
    u8 data[MAX_DATA_SIZE];

};

// Maps
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct config);
} config_map SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

// Helper function to check if fd is in target list
static __always_inline int is_target_fd(struct config *cfg, u32 fd) {
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
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    u32 tid = (u32)pid_tgid;
    
    // Get configuration
    u32 key = 0;
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
    u64 fd = ctx->args[0];
    const char *buf = (const char*)ctx->args[1];
    size_t count = (size_t)ctx->args[2];
    
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
    u32 data_size = count < MAX_DATA_SIZE ? count : MAX_DATA_SIZE;
    event->count = data_size;
    event->timestamp = bpf_ktime_get_ns(); // time 
    // get the current name of the process
    bpf_get_current_comm(event->comm, sizeof(event->comm));

    // Read the data from the user-space buffer.
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
// go:build ignore
//  +build ignore

#include "vmlinux.h"
#include <bpf/bpf_core_read.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <linux/version.h>

// Get the kernel version and set variadic variable for bpf_printk
#if LINUX_VERSION_CODE >= KERNEL_VERSION(5, 16, 0)
  #define BPF_PRINTK_VARIADIC 1
#endif

// Event capture configuration
#define MAX_FDS 64            // maximum number of file descriptors to filter
#define MAX_DATA_SIZE 256     // max bytes retrieved from the write buffer
#define MAX_EXEC_NAME_SIZE 16 // max size of the program name (task_struct->comm)

// Ring buffer configuration
// 256KB provides enough space for ~1000 concurrent write events
// assuming average event size of ~256 bytes (sizeof(write_event))
#define RINGBUF_SIZE (256 * 1024)

// Maximum number of threads/processes that can be tracked simultaneously
// Set to support large parallel applications (e.g., MPI jobs with 10k ranks)
#define MAX_TRACKED_THREADS 10240

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
  __uint(max_entries, RINGBUF_SIZE);
} events SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, MAX_TRACKED_THREADS);
  __type(key, __u32);
  __type(value, __u32);
} tracked_pids SEC(".maps");

// Helper function to check if fd is in target list
static __always_inline int is_target_fd(struct config *cfg, __u32 fd) {
  for (int i = 0; i < MAX_FDS; i++) {
    if (i >= cfg->num_fds)
      break;
    if (cfg->target_fds[i] == fd) {
      return 1;
    }
  }
  return 0;
}

SEC("tracepoint/syscalls/sys_enter_write")
int trace_write_enter(struct trace_event_raw_sys_enter *ctx) {
  __u64 pid_tgid = bpf_get_current_pid_tgid();
  __u32 pid = pid_tgid >> 32;  // This is actually TGID
  __u32 tid = (__u32)pid_tgid; // This is TID

  // Get configuration
  __u32 key = 0;
  struct config *cfg = bpf_map_lookup_elem(&config_map, &key);
  if (!cfg) {
    return 0;
  }

  // Check if this is a tracked thread
  __u32 *tracked = bpf_map_lookup_elem(&tracked_pids, &tid);
  if (!tracked) {
    return 0;
  }

  // The arguments to the write syscall are in the `ctx->args` array.
  // args[0] is the file descriptor (fd).
  // args[1] is the user-space buffer (buf).
  // args[2] is the count of bytes to write.
  __u64 fd = ctx->args[0];
  const char *buf = (const char *)ctx->args[1];
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
  event->pid = pid;     // process ID
  event->tid = tid;     // thread ID
  event->fd = fd;       // file descriptor
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

  // Changing to two separate print statements: https://github.com/Perif/write-tracer/issues/2
  #ifdef BPF_PRINTK_VARIADIC
    // Variadic bpf_printk - kernel version 5.16 or later (https://docs.ebpf.io/ebpf-library/libbpf/ebpf/bpf_printk/)
    bpf_printk(
        "trace_write_enter pid %d tid %d fd %d count %llu name %s data %s\n",
        event->pid, event->tid, event->fd, event->count, (char *)event->comm,
        (char *)event->data);
  #else
    bpf_printk("trace_write_enter: pid=%d tid=%d fd=%d", event->pid, event->tid, event->fd);
    bpf_printk("[cont.] trace_write_enter: count=%llu comm=%s", event->count, (char *)event->comm);
  #endif
  // #endif
    
  // Submit event
  bpf_ringbuf_submit(event, 0);

  return 0;
}

SEC("raw_tracepoint/sched_process_fork")
int trace_sched_process_fork(struct bpf_raw_tracepoint_args *ctx) {
  struct task_struct *parent = (struct task_struct *)ctx->args[0];
  struct task_struct *child = (struct task_struct *)ctx->args[1];

  __u32 parent_tid = BPF_CORE_READ(parent, pid);
  __u32 child_tid = BPF_CORE_READ(child, pid);

  // If parent thread is tracked, track the child thread (or process) as well
  __u32 *tracked = bpf_map_lookup_elem(&tracked_pids, &parent_tid);
  if (tracked) {
    __u32 val = 1;
    bpf_map_update_elem(&tracked_pids, &child_tid, &val, BPF_ANY);
    bpf_printk("fork: parent tid %d tracked, tracking child tid %d\n",
               parent_tid, child_tid);
  }

  return 0;
}

SEC("raw_tracepoint/sched_process_exit")
int trace_sched_process_exit(struct bpf_raw_tracepoint_args *ctx) {
  struct task_struct *task = (struct task_struct *)ctx->args[0];
  __u32 tid = BPF_CORE_READ(task, pid);

  // Stop tracking this specific thread when it exits
  bpf_map_delete_elem(&tracked_pids, &tid);

  return 0;
}

char LICENSE[] SEC("license") = "GPL";

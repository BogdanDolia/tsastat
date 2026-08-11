// SPDX-License-Identifier: (GPL-2.0-only OR Apache-2.0)

typedef unsigned char __u8;
typedef unsigned int __u32;
typedef unsigned long long __u64;

#define SEC(name) __attribute__((section(name), used))
#define __always_inline inline __attribute__((always_inline))
#define __uint(name, value) int (*name)[value]
#define __type(name, value) value *name
#define CORE_FIELD(field) __builtin_preserve_access_index(field)

#define BPF_MAP_TYPE_ARRAY 2
#define BPF_MAP_TYPE_RINGBUF 27

#define EVENT_WAKEUP 1
#define EVENT_SWITCH_IN 2
#define EVENT_SWITCH_OUT 3

#define EVENT_FLAG_PREEMPTED 1

struct task_struct {
	int pid;
	int tgid;
	char comm[16];
	__u64 start_time;
} __attribute__((preserve_access_index));

struct scheduler_event {
	__u64 timestamp_ns;
	__u64 start_time_ns;
	__u64 state;
	__u32 tgid;
	__u32 tid;
	__u32 cpu;
	__u32 type;
	__u32 flags;
	char comm[16];
	__u32 padding;
};

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u32);
} config SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u64);
} lost_events SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 22);
} events SEC(".maps");

static void *(*bpf_map_lookup_elem)(void *map, const void *key) = (void *)1;
static __u64 (*bpf_ktime_get_ns)(void) = (void *)5;
static __u32 (*bpf_get_smp_processor_id)(void) = (void *)8;
static long (*bpf_probe_read_kernel)(void *dst, __u32 size, const void *unsafe_ptr) = (void *)113;
static void *(*bpf_ringbuf_reserve)(void *ringbuf, __u64 size, __u64 flags) = (void *)131;
static void (*bpf_ringbuf_submit)(void *data, __u64 flags) = (void *)132;

static __always_inline void count_lost_event(void)
{
	__u32 key = 0;
	__u64 *count = bpf_map_lookup_elem(&lost_events, &key);

	if (count)
		__sync_fetch_and_add(count, 1);
}

static __always_inline int read_task_identity(struct task_struct *task,
					       __u32 *tgid, __u32 *tid,
					       __u64 *start_time_ns)
{
	if (bpf_probe_read_kernel(tgid, sizeof(*tgid), CORE_FIELD(&task->tgid)) < 0)
		return -1;
	if (bpf_probe_read_kernel(tid, sizeof(*tid), CORE_FIELD(&task->pid)) < 0)
		return -1;
	if (bpf_probe_read_kernel(start_time_ns, sizeof(*start_time_ns),
				  CORE_FIELD(&task->start_time)) < 0)
		return -1;
	return 0;
}

static __always_inline int emit_task_event(struct task_struct *task, __u32 type,
					    __u64 state, __u32 flags,
					    __u64 timestamp_ns)
{
	__u32 key = 0;
	__u32 *target_tgid = bpf_map_lookup_elem(&config, &key);
	__u32 tgid = 0;
	__u32 tid = 0;
	__u64 start_time_ns = 0;
	struct scheduler_event *event;

	if (!target_tgid)
		return 0;
	if (read_task_identity(task, &tgid, &tid, &start_time_ns) < 0)
		return 0;
	if (tgid != *target_tgid)
		return 0;

	event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
	if (!event) {
		count_lost_event();
		return 0;
	}

	event->timestamp_ns = timestamp_ns;
	event->start_time_ns = start_time_ns;
	event->state = state;
	event->tgid = tgid;
	event->tid = tid;
	event->cpu = bpf_get_smp_processor_id();
	event->type = type;
	event->flags = flags;
	event->padding = 0;
	__builtin_memset(event->comm, 0, sizeof(event->comm));
	bpf_probe_read_kernel(event->comm, sizeof(event->comm), CORE_FIELD(&task->comm));
	bpf_ringbuf_submit(event, 0);
	return 0;
}

SEC("tp_btf/sched_switch")
int handle_sched_switch(__u64 *ctx)
{
	__u32 preempted = (__u32)ctx[0];
	struct task_struct *prev = (struct task_struct *)ctx[1];
	struct task_struct *next = (struct task_struct *)ctx[2];
	__u64 prev_state = ctx[3];
	__u64 timestamp_ns = bpf_ktime_get_ns();

	emit_task_event(prev, EVENT_SWITCH_OUT, prev_state,
			preempted ? EVENT_FLAG_PREEMPTED : 0, timestamp_ns);
	emit_task_event(next, EVENT_SWITCH_IN, 0, 0, timestamp_ns);
	return 0;
}

SEC("tp_btf/sched_wakeup")
int handle_sched_wakeup(__u64 *ctx)
{
	struct task_struct *task = (struct task_struct *)ctx[0];

	return emit_task_event(task, EVENT_WAKEUP, 0, 0, bpf_ktime_get_ns());
}

SEC("tp_btf/sched_wakeup_new")
int handle_sched_wakeup_new(__u64 *ctx)
{
	struct task_struct *task = (struct task_struct *)ctx[0];

	return emit_task_event(task, EVENT_WAKEUP, 0, 0, bpf_ktime_get_ns());
}

char LICENSE[] SEC("license") = "GPL";

**tsastat project review and improvement plan — September 8, 2026**

Review baseline: [`8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c`](https://github.com/BogdanDolia/tsastat/commit/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c), the `main` branch on September 8, 2026. The review covered the CLI, procfs, taskstats/Netlink, eBPF C programs and Go stream, both accumulators, hybrid merging, JSON/table output, doctor, tests, CI/release/Pages workflows, and the published browser viewer. Source code was not modified during the analysis. Experimental checks ran in a separate temporary copy of the project. This document describes the specified commit; the fixes and new features listed below have not been implemented.

Seven reproducible defects were found: one P1 and six P2 issues. The most significant problem is numerical precision loss during export, which hides thread activity in the viewer. No P0 issues were identified within the reviewed scope. The passing standard tests do not cover the scenarios described below. Reproductions include synthetic Go inputs, Node checks, and browser interactions; they do not establish that all seven defects were observed during live Linux collection.

At the time of the review, the published [GitHub Pages website](https://bogdandolia.github.io/tsastat/) was accessible. The latest [Pages deployment](https://github.com/BogdanDolia/tsastat/actions/runs/31508922349) had succeeded and matched the reviewed SHA. The latest release at that time was `v0.1.0`. Five Dependabot PRs, #14–18, were open; their changes are outside this review of `main`.

1. **[P1] JSON truncates nonzero scheduler metrics to zero.**

Location: [internal/output/json.go:290](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/internal/output/json.go#L290), [wakeup conversion:301](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/internal/output/json.go#L301), [viewer sorting:205](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/site/app.js#L205).

`time.Duration.Milliseconds()` discards fractional milliseconds from CPU/RQ durations, and `Microseconds()` discards fractional microseconds from wakeup latency. A synthetic `ThreadIntervalStats` value with CPU=900 µs, RQ=800 µs, and wakeup=800 ns was passed to the actual Go JSON renderer and serialized as `on_cpu_ms=0`, `runqueue_wait_ms=0`, and `wakeup_latency_avg_us=0`. Meanwhile, `on_cpu_percent=0.09` retained evidence of the nonzero input in the same export. This isolates precision loss in serialization; it is not a measurement of live collector accuracy. The viewer uses the truncated values for its table, sorting, and bar widths. Threads with short CPU/RQ durations become visually indistinguishable from threads with no activity. Code inspection also shows that windows shorter than one millisecond lose precision in `interval_ms` and coverage fields.

Proposed fix: add exact duration fields in nanoseconds, or introduce fractional milliseconds through a versioned schema. Adding fields while retaining the existing ones is safer for compatibility. Round only the displayed text; sort and aggregate the original values. Regression coverage should include nonzero values below 1 ms/1 µs, different short intervals, and preservation of totals after export.

2. **[P2] State for exited eBPF threads remains in memory.**

Location: [internal/collector/event_accumulator.go:179](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/internal/collector/event_accumulator.go#L179), [exit handling:235](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/internal/collector/event_accumulator.go#L235).

`SchedulerStateDead` sets `alive=false`, but the entry is never removed from `tracks`. `Advance()` continues traversing the entire map, merely skipping dead entries. A unit-level reproduction supplied synthetic switch-in and switch-out-to-dead events for 10,000 distinct TIDs directly to the accumulator. After advancing through the relevant windows, the map still contained 10,000 entries. This confirms retained state and continued traversal of historical TIDs. It did not create 10,000 Linux threads or benchmark heap size, RSS, or CPU overhead. Increased memory and iteration cost during long-running thread churn are consequences of the retained map; the size of that overhead remains unmeasured. TID reuse can limit map growth.

Proposed fix: release completed tracks after safely advancing the watermark and preserving their final window statistics. Test late events, TID reuse, exit at a window boundary, and sustained thread churn.

3. **[P2] Thread names reach the terminal with control characters intact.**

Location: [internal/output/table.go:38](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/internal/output/table.go#L38).

`stat.Comm` is passed to tabwriter without escaping. A renderer test captured output in a byte buffer using the synthetic name `worker\x1b[2J\nX`; the ESC sequence and embedded newline remained in the output. A terminal may interpret screen-control sequences, while tabs and newlines alter the report structure. The test confirmed unsafe output bytes; it did not execute terminal controls interactively or establish arbitrary shell-command execution.

Proposed fix: render control characters as visible escape sequences in the table renderer. Preserve the original name in the model and correctly escaped JSON. Test ESC, CR/LF, tabs, Unicode, and long names.

4. **[P2] The viewer displays unmeasured wakeup latency as zero.**

Location: [site/app.js:172](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/site/app.js#L172), [WAKE AVG column:362](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/site/app.js#L362).

The viewer checks the general `scheduler.available` flag, which means schedstat is available for the proc backend. Wakeup latency requires an event backend. Proc exports contain `event_timed=false` and zero-valued wakeup fields, so the website displays `WAKE AVG=0 µs`. This was reproduced on the published website using a synthetic proc-shaped input serialized by the actual Go renderer. The fixture was not a live proc capture. The CLI table already handles the unavailable metric correctly by checking `SchedulerSource == ebpf_sched_events`.

Proposed fix: determine availability separately for each metric. For wakeup latency, consider event timing and the number of measurements. The table, tooltip, and sorting behavior must distinguish no measurement from a measured zero.

5. **[P2] The chart replaces unavailable CPU/RQ metrics with 0 ms.**

Location: [site/app.js:291](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/site/app.js#L291).

When `scheduler.available=false`, the table shows dashes, but the bar label is calculated as `(cpu ?? 0) + (runqueue ?? 0)` and displays `0 ms`. This availability condition can occur with the explicit taskstats backend or proc without accessible schedstat data. The browser reproduction used a copy of the synthetic proc fixture with `backend=taskstats` and `scheduler.available=false`; it was not a live taskstats capture. On the published website, the accessibility label `— CPU, — runqueue`, an empty bar, and a value of `0ms` appeared together.

Proposed fix: preserve unavailability throughout the presentation. If both metrics are absent, show a “Scheduler metrics unavailable” state. If availability is partial, do not present the sum as complete. Apply the same principle to delay aggregates by showing how many visible threads are actually covered by measurements.

6. **[P2] A failed import can corrupt the current viewer state.**

Location: [site/app.js:153](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/site/app.js#L153), [loadReports:404](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/site/app.js#L404).

The parser checks for a `threads` array but does not validate its elements. `{"threads":[null]}` passes `parseReport()`. Then `loadReports()` replaces `state.reports` before validating identities and fails when reading `thread.tid`. The previous table remains visible even though its state has been replaced. Typing into the filter causes an unhandled `TypeError: Cannot read properties of null (reading 'comm')` at `app.js:220`. Both stages were reproduced in the published viewer. Loading another valid report restores operation.

Proposed fix: fully validate and normalize the new document before changing the working state. Validate thread entries, identity types, intervals, metric values, and nested objects separately. On failure, preserve the previous report and show a clear line/thread reference. Add a regression test for “valid report → invalid import → the previous report's filter still works.”

7. **[P2] The Threads count merges distinct TID lifetimes when start_time_ticks is zero.**

Location: [site/app.js:414](https://github.com/BogdanDolia/tsastat/blob/8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c/site/app.js#L414).

The identity key uses `start_time_ticks ?? start_time_ns`. However, `0` can indicate an unavailable proc identity, and `??` does not fall back for zero. For two records with the same TID and `start_time_ticks=0`, but `start_time_ns=100` and `200`, the summary's identity expression counts one thread instead of two. A Node reproduction evaluated that expression against synthetic records; live PID/TID reuse was not exercised. The finding is limited to the incorrect summary count; this code does not add CPU values from those rows together.

Proposed fix: choose a proven nonzero identity and include PID/session identity. Do not fabricate a reliable identity when both start-time fields are unknown. Prefer strings for long identifiers to avoid JavaScript Number precision limits.

**Fix checklist and acceptance criteria**

All items below are open. P1 identifies a priority fix for result accuracy; P2 identifies a confirmed defect to address in planned work. Implementation order accounts for the viewer's dependency on the JSON contract.

- [ ] B1 / P1 — preserve export precision: CPU=900 µs, RQ=800 µs, and wakeup=800 ns remain distinguishable after a JSON round trip; charts, sorting, and totals use exact values; compatibility with existing reports is verified.
- [ ] B2 / P2 — release dead tracks: after exit and window finalization, memory use no longer depends on historical thread count; late events and TID reuse do not revive a completed series.
- [ ] B3 / P2 — escape control characters in table output: ESC, CR/LF, and tabs in `comm` do not alter table structure; ordinary Unicode names are preserved.
- [ ] B4 / P2 — treat wakeup availability separately from schedstat: proc/fallback reports show no measurement, while eBPF reports with measurements show the correct value.
- [ ] B5 / P2 — preserve unavailability in charts: absent CPU/RQ metrics do not become zero; partial aggregate coverage is identified.
- [ ] B6 / P2 — make imports atomic: `{"threads":[null]}` is rejected before replacing state, the previous report and its filter remain functional, and the error identifies the invalid record.
- [ ] B7 / P2 — correct the identity key: the same TID with zero ticks and different nonzero start-time nanoseconds represents two thread lifetimes; unknown identity is not presented as proven.
- [ ] Q1 — add CLI → viewer CI checks using JSON from the current Go renderer: normal reports, fallback, unavailable metrics, sub-millisecond values, empty results, malformed imports, and TID reuse; add browser checks for the main interactions.
- [ ] Q2 — separately run privileged Linux checks for eBPF/taskstats: attach, loss/late quality, churn, window boundaries, exit, host/time namespaces, and delayacct. Record the kernel, permissions, workload, and actual results.

**Functional improvements to prioritize**

The viewer is useful for inspecting an individual interval, but it is insufficient for investigating process behavior over time. Its main screen omits much of the data already collected. Improve report analysis before adding another backend.

| Priority | Change | Practical outcome |
| --- | --- | --- |
| First phase | Fix the seven findings and add JSON/viewer CI checks | Reliable metrics and predictable import behavior |
| First phase | A report-wide timeline for CPU, RQ, wakeup/resource delays, D-state, and quality, with navigation to peak intervals | Find CPU, contention, and latency spikes without moving through every slider position |
| First phase | A selected-thread detail view with all intervals, state-duration bars, wakeup avg/max/count, and all delay subtypes | Inspect rare delays, thread lifetime, and causes of waiting |
| First phase | A complete quality and coverage panel for each source | Understand which parts of a report support a given conclusion |
| Second phase | Process summary and selection of multiple intervals | Analyze the whole process and a workload segment |
| Second phase | Compare two captures or a baseline and workload | Quantify regressions |
| Second phase | Streaming JSONL parsing, a Web Worker, and table virtualization | Reduce UI blocking for large reports and allow a higher limit than 20 MB |
| Second phase | Recover complete records when the final JSONL line is truncated, with an explicit warning | Preserve the analytical value of interrupted captures |
| Second phase | An optional final partial CLI interval | Retain the observed segment in short captures and after Ctrl-C |
| Later | `--columns`, a compact table preset, implemented CSV output, and process-tree/cgroup monitoring | Improve terminal use, scripting, and analysis of services spanning multiple processes |

Aggregates require correct denominators: total process CPU usage can be expressed in equivalent cores, and average wakeup latency should be calculated from total latency and event count. Taking the arithmetic mean of per-thread averages produces an incorrect result. Percentiles cannot be reconstructed from exported averages and maxima; they require histograms or individual events.

The quality panel should show active/unavailable sources, the specific fallback reason, lost/late events, initialization races, incomplete wakeups, identity mismatches, actual maximum scan/sample gaps, resets, and scheduler/delay coverage. Currently, `qualityWarnings()` does not list `initialization_races` or sample-gap values. The `missed_transitions_possible` warning always refers to sampling, even for an incomplete eBPF stream. When warnings appear, the active-source list disappears. These are additional limitations in explaining data quality, rather than separately reproduced collector defects.

A state viewer is particularly important for taskstats and proc without schedstat: missing scheduler values leave the main chart empty even when sampled state durations are present in JSON. D-state must not automatically be labeled as proven I/O wait. Taskstats measures specific resource delays, including synchronous block I/O, and exposes counters through queries during a task's lifetime and through exit records. The current implementation uses polling only. Complete accounting for short-lived threads requires consuming exit records, as described in the [Linux delay accounting documentation](https://docs.kernel.org/accounting/delay-accounting.html).

The CLI checks showed that a target exiting and being reaped after approximately 400 ms, before the first two-second window, produces exit code 1 and no JSON lines. SIGINT after approximately 400 ms produces exit code 0 and no lines. A proposed partial report must state its actual boundaries, coverage, and termination reason; it must not be presented as a full two-second measurement. This is a proposal for new behavior, not an instruction to silently change the semantics of the existing count option.

For Pages, useful additions include a short interpretation guide for CPU-bound work, runqueue contention, and I/O wait; a link to the relevant demo interval; an explicit time zone and date; previous/next interval keyboard controls; quick peak selection; and separate demos for proc, taskstats, fallback, and lost events. The existing palette and system theme can be retained. The summary occupies substantial space on mobile, so a compact summary grid and expandable details would improve usability without a full redesign.

**Verification and observed results**

The table records the original review run. During this documentation correction, the temporary reproduction copy was verified against every tracked file at the reviewed SHA, and the targeted Go and Node probes were rerun with the same results. The full standard suite, Docker runtime checks, and published-browser checks were not rerun for this documentation change.

| Check | Result |
| --- | --- |
| Local HEAD and GitHub main at review time | Matched: `8d0c2bd93d450d2b1e4c61d739d0ed3afe591a7c` |
| `go test -race -covermode=atomic ./...` | All standard tests passed on macOS arm64 / Go 1.26.1 |
| Coverage from that run | 65.6% of statements; collector 81.1%; excludes execution of the Linux-only stream |
| `go vet ./...`, `go mod verify`, `node --check site/app.js`, `git diff --check` | Passed |
| Linux amd64 and arm64 builds | Passed; ELF outputs verified |
| Linux test binaries for 11 packages | All passed in a Docker VM running kernel `7.0.12-linuxkit` |
| Live proc | 13 threads, three 200 ms windows, exit code 0 |
| Live auto without privileges | Three windows, `active_sources=[proc]`, `unavailable_sources=[ebpf,taskstats]` |
| Explicit taskstats/eBPF without privileges | Expected permission errors; no successful capture was reported |
| Separate Go probes with synthetic inputs | B1: nonzero durations exported as zero; B2: 10,000 dead tracks retained; B3: raw ESC/newline bytes; B4 fixture: proc schedstat available, event timing absent, wakeup zero |
| Viewer checks in Node | B4: wakeup helper returns zero for the proc fixture; B6: parser accepts a null thread and filtering throws; B7: the identity expression counts one lifetime instead of two |
| Published viewer in a browser | Demo, slider, and filter checked; B4/B5 reproduced using synthetic imports; B6 reproduced by loading a valid report, importing a null thread, and using the filter |
| Mobile widths of 390 and 360 px | No horizontal page overflow; the table has its own scrolling area |
| Repository state during the original analysis | Source code was unchanged |

Three Go probes asserted the expected correct behavior and failed on B1, B2, and B3. The proc-availability Go probe passed while logging the fixture's values. The Node checks logged observed values and caught exceptions, so their process exited successfully even though they demonstrated defects. These probes have not been added to the standard test suite; a passing standard CI run does not mean the findings are fixed. Findings 1–7 describe the scenarios, input values, and observed results. Implementing the fixes should include turning these scenarios into permanent regression tests.

CI currently runs Go formatting, vet, tests, builds, and a GoReleaser snapshot, but no JS unit/browser tests or CLI → viewer contract checks. The Pages workflow uploads `site` directly. Existing CI cannot detect the website defects described here. The minimum next set of checks should cover fixtures from every backend, sub-millisecond metrics, unavailable data, PID/TID reuse, malformed imports that preserve the previous state, XSS/control strings, empty results, keyboard navigation, and mobile widths. Validate against JSON generated by the current Go renderer, not only a hand-authored demo.

**Remaining review limitations**

A full live eBPF/taskstats capture with elevated permissions was not performed. Automatic approval review rejected the proposed Docker run with the host PID namespace, BPF/PERFMON/NET_ADMIN capabilities, and unconfined seccomp because it could expose processes and telemetry outside the test. That run did not execute. Linux unit tests and isolated unprivileged runtime checks were performed instead.

The successful Linux builds were Go builds using the repository's existing embedded BPF objects; no fresh compilation of the BPF C programs is claimed. Building the CLI and passing unit tests do not establish that BPF programs can be loaded, attached, or produce accurate live measurements. The remaining privileged checks are unverified, rather than tests that ran and failed.

Consequently, this review does not establish live tracing behavior across kernels, ring-buffer loss under load, sustained churn with real eBPF events, time-namespace offsets, or taskstats accuracy with delayacct enabled. These require separate short tests in a dedicated Linux environment with explicit tracing authorization.

One additional hypothesis for a load test is whether flush/watermark processing remains bounded. The collector waits for `Flushed()` without its own deadline, and the current ringbuf reader reaches the flush notification through completion of reading available events. Under a continuous high event rate, check for starvation and increasing report-emission latency. This is a risk identified through code analysis, not a reproduced defect. The [BPF ring buffer documentation](https://docs.kernel.org/bpf/ringbuf.html) describes reservation/commit ordering and possible consumer delays; it does not, by itself, prove that this application hangs.

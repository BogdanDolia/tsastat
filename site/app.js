"use strict";

const THEME_KEY = "tsastat-theme";
const THEME_ORDER = ["system", "light", "dark"];
const MAX_REPORT_BYTES = 20 * 1024 * 1024;

const $ = (selector, root = document) => root.querySelector(selector);

function readThemePreference() {
  try {
    const saved = localStorage.getItem(THEME_KEY);
    return THEME_ORDER.includes(saved) ? saved : "system";
  } catch (_) {
    return "system";
  }
}

function applyTheme(theme) {
  const control = $("#theme-control");
  const label = $("#theme-control-label");
  const icon = control?.querySelector(".theme-control-icon");
  const themeColors = document.querySelectorAll('meta[name="theme-color"]');

  if (theme === "system") {
    delete document.documentElement.dataset.theme;
  } else {
    document.documentElement.dataset.theme = theme;
  }

  themeColors.forEach((meta) => {
    if (theme === "system") {
      meta.content = meta.media.includes("dark") ? "#0d121c" : "#f4f7fb";
    } else {
      meta.content = theme === "dark" ? "#0d121c" : "#f4f7fb";
    }
  });

  try {
    localStorage.setItem(THEME_KEY, theme);
  } catch (_) {}

  const nextTheme = THEME_ORDER[(THEME_ORDER.indexOf(theme) + 1) % THEME_ORDER.length];
  const labels = { system: "Theme: system", light: "Theme: light", dark: "Theme: dark" };
  const icons = { system: "◐", light: "☼", dark: "☾" };

  if (label) label.textContent = labels[theme];
  if (icon) icon.textContent = icons[theme];
  if (control) {
    control.dataset.themeMode = theme;
    control.setAttribute("aria-label", `${labels[theme]}. Switch to ${nextTheme} theme.`);
    control.title = `${labels[theme]}. Click for ${nextTheme}.`;
  }
}

function initializeTheme() {
  let theme = readThemePreference();
  applyTheme(theme);

  $("#theme-control")?.addEventListener("click", () => {
    theme = THEME_ORDER[(THEME_ORDER.indexOf(theme) + 1) % THEME_ORDER.length];
    applyTheme(theme);
  });
}

async function copyText(text, button) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (_) {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.append(textarea);
    textarea.select();
    document.execCommand("copy");
    textarea.remove();
  }

  const previous = button.textContent;
  button.textContent = "Copied";
  window.setTimeout(() => {
    button.textContent = previous;
  }, 1600);
}

function initializeCopyButtons() {
  document.querySelectorAll("[data-copy]").forEach((button) => {
    button.addEventListener("click", () => copyText(button.dataset.copy, button));
  });
}

function numeric(value, fallback = 0) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
}

function metricOrNull(value) {
  if (value === null || value === undefined || value === "") return null;
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : null;
}

function formatMetric(value, unit, digits = 1) {
  if (value === null || value === undefined || !Number.isFinite(value)) return "—";
  const absolute = Math.abs(value);
  const precision = absolute >= 100 || Number.isInteger(value) ? 0 : digits;
  return `${value.toFixed(precision)}${unit}`;
}

function formatTime(timestamp) {
  if (!timestamp) return "Unknown time";
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) return String(timestamp);
  return new Intl.DateTimeFormat(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: 3,
  }).format(date);
}

function createElement(tag, className, text) {
  const element = document.createElement(tag);
  if (className) element.className = className;
  if (text !== undefined) element.textContent = text;
  return element;
}

function parseReport(text) {
  const trimmed = text.trim();
  if (!trimmed) throw new Error("The report is empty.");

  let records;
  if (trimmed.startsWith("[")) {
    const parsed = JSON.parse(trimmed);
    records = Array.isArray(parsed) ? parsed : [parsed];
  } else {
    records = trimmed
      .split(/\r?\n/)
      .filter((line) => line.trim())
      .map((line, index) => {
        try {
          return JSON.parse(line);
        } catch (error) {
          throw new Error(`Line ${index + 1} is not valid JSON: ${error.message}`);
        }
      });
  }

  if (!records.length) throw new Error("No reporting intervals were found.");

  records.forEach((record, index) => {
    if (!record || typeof record !== "object" || Array.isArray(record)) {
      throw new Error(`Interval ${index + 1} is not a JSON object.`);
    }
    if (!Array.isArray(record.threads)) {
      throw new Error(`Interval ${index + 1} has no threads array.`);
    }
  });

  return records;
}

function delayMS(thread, key) {
  const counter = thread?.delays?.[key];
  if (!counter || counter.available === false) return null;
  const totalNS = metricOrNull(counter.total_ns);
  return totalNS === null ? null : totalNS / 1_000_000;
}

function schedulerMetric(thread, key) {
  if (!thread?.scheduler || thread.scheduler.available === false) return null;
  return metricOrNull(thread.scheduler[key]);
}

function qualityWarnings(report) {
  const quality = report.quality || {};
  const warnings = [];
  const unavailable = Array.isArray(quality.unavailable_sources) ? quality.unavailable_sources : [];

  if (unavailable.length) warnings.push(`Unavailable sources: ${unavailable.join(", ")}`);
  if (numeric(quality.scheduler_lost_events_total) > 0) {
    warnings.push(`${numeric(quality.scheduler_lost_events_total)} scheduler events lost`);
  }
  if (numeric(quality.scheduler_late_events) > 0) {
    warnings.push(`${numeric(quality.scheduler_late_events)} late scheduler events`);
  }
  if (numeric(quality.scheduler_incomplete_wakeups) > 0) {
    warnings.push(`${numeric(quality.scheduler_incomplete_wakeups)} incomplete wakeups`);
  }
  if (numeric(quality.hybrid_identity_mismatches) > 0) {
    warnings.push(`${numeric(quality.hybrid_identity_mismatches)} identity mismatches`);
  }
  if (quality.missed_transitions_possible === true) warnings.push("State transitions may be missed between samples");
  if (numeric(quality.schedstat_counter_resets) > 0) warnings.push("schedstat counter resets detected");
  if (numeric(quality.taskstats_counter_resets) > 0) warnings.push("taskstats counter resets detected");
  if (quality.kernel_task_delayacct_enabled_known === true && quality.kernel_task_delayacct_enabled === false) {
    warnings.push("Kernel task delay accounting is disabled");
  }

  return warnings;
}

function threadSortValue(thread, sort) {
  switch (sort) {
    case "runqueue": return schedulerMetric(thread, "runqueue_wait_ms") ?? -1;
    case "wakeup": return schedulerMetric(thread, "wakeup_latency_avg_us") ?? -1;
    case "io": return delayMS(thread, "block_io") ?? -1;
    case "tid": return numeric(thread.tid, Number.MAX_SAFE_INTEGER);
    case "cpu":
    default: return schedulerMetric(thread, "on_cpu_ms") ?? -1;
  }
}

function getVisibleThreads(report, query, sort) {
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const threads = report.threads.filter((thread) => {
    if (!normalizedQuery) return true;
    return String(thread.comm || "").toLocaleLowerCase().includes(normalizedQuery)
      || String(thread.tid ?? "").includes(normalizedQuery);
  });

  return threads.sort((a, b) => {
    const aValue = threadSortValue(a, sort);
    const bValue = threadSortValue(b, sort);
    if (sort === "tid") return aValue - bValue;
    return bValue - aValue || numeric(a.tid) - numeric(b.tid);
  });
}

function renderQuality(report) {
  const panel = $("#quality-panel");
  const warnings = qualityWarnings(report);
  panel.replaceChildren();
  panel.classList.toggle("has-warnings", warnings.length > 0);

  if (!warnings.length) {
    const sources = Array.isArray(report.quality?.active_sources) && report.quality.active_sources.length
      ? report.quality.active_sources.join(", ")
      : report.backend || "reported backend";
    const message = createElement("span");
    const strong = createElement("strong", "", "No explicit completeness warnings. ");
    message.append(strong, `Active sources: ${sources}.`);
    panel.append(message);
    return;
  }

  const content = createElement("div");
  content.append(createElement("strong", "", "Review quality before interpreting this interval."));
  const list = createElement("ul");
  warnings.forEach((warning) => list.append(createElement("li", "", warning)));
  content.append(list);
  panel.append(content);
}

function renderThreadBars(threads, report) {
  const container = $("#thread-bars");
  container.replaceChildren();
  const displayThreads = threads.slice(0, 7);
  const maxValue = Math.max(
    ...displayThreads.map((thread) => (schedulerMetric(thread, "on_cpu_ms") ?? 0) + (schedulerMetric(thread, "runqueue_wait_ms") ?? 0)),
    numeric(report.interval_ms, 1),
    1,
  );

  if (!displayThreads.length) {
    container.append(createElement("p", "delay-empty", "No threads match the current filter."));
    return;
  }

  displayThreads.forEach((thread) => {
    const cpu = schedulerMetric(thread, "on_cpu_ms");
    const runqueue = schedulerMetric(thread, "runqueue_wait_ms");
    const cpuWidth = Math.max(0, ((cpu ?? 0) / maxValue) * 100);
    const rqWidth = Math.max(0, ((runqueue ?? 0) / maxValue) * 100);

    const row = createElement("div", "thread-bar-row");
    const label = createElement("div", "thread-bar-label");
    label.append(createElement("strong", "", thread.comm || "unnamed"));
    label.append(createElement("span", "", `TID ${thread.tid ?? "—"}`));

    const track = createElement("div", "thread-bar-track");
    track.setAttribute("aria-label", `${thread.comm || "Thread"}: ${formatMetric(cpu, "ms")} CPU, ${formatMetric(runqueue, "ms")} runqueue`);
    const cpuBar = createElement("span", "thread-bar-cpu");
    const rqBar = createElement("span", "thread-bar-rq");
    cpuBar.style.setProperty("--cpu", `${cpuWidth}%`);
    rqBar.style.setProperty("--rq", `${rqWidth}%`);
    track.append(cpuBar, rqBar);

    const value = createElement("span", "thread-bar-value", formatMetric((cpu ?? 0) + (runqueue ?? 0), "ms"));
    row.append(label, track, value);
    container.append(row);
  });
}

function renderDelaySummary(threads) {
  const container = $("#delay-summary");
  container.replaceChildren();
  const definitions = [
    ["CPU", "cpu"],
    ["Block I/O", "block_io"],
    ["Swap in", "swap_in"],
    ["Reclaim", "reclaim"],
    ["Thrashing", "thrashing"],
    ["Compaction", "compaction"],
    ["WP copy", "write_protect_copy"],
    ["IRQ", "irq"],
  ];

  const values = definitions.map(([label, key]) => {
    let available = false;
    const total = threads.reduce((sum, thread) => {
      const value = delayMS(thread, key);
      if (value !== null) available = true;
      return sum + (value ?? 0);
    }, 0);
    return { label, total, available };
  });
  const availableValues = values.filter((item) => item.available);

  if (!availableValues.length) {
    container.append(createElement("p", "delay-empty", "No taskstats delay counters are available for these threads."));
    return;
  }

  const max = Math.max(...availableValues.map((item) => item.total), 0.001);
  availableValues.forEach((item) => {
    const row = createElement("div", "delay-row");
    const label = createElement("span", "", item.label);
    const track = createElement("span", "delay-track");
    const bar = createElement("i");
    bar.style.setProperty("--value", `${Math.max(1.5, (item.total / max) * 100)}%`);
    track.append(bar);
    const value = createElement("span", "", formatMetric(item.total, "ms", 2));
    row.append(label, track, value);
    container.append(row);
  });
}

function renderThreadTable(threads) {
  const body = $("#thread-table-body");
  body.replaceChildren();
  $("#visible-thread-count").textContent = `${threads.length} ${threads.length === 1 ? "thread" : "threads"}`;

  if (!threads.length) {
    const row = document.createElement("tr");
    const cell = createElement("td", "metric-unavailable", "No threads match the current filter.");
    cell.colSpan = 7;
    row.append(cell);
    body.append(row);
    return;
  }

  threads.forEach((thread) => {
    const row = document.createElement("tr");
    const values = [
      thread.comm || "unnamed",
      thread.tid ?? "—",
      formatMetric(schedulerMetric(thread, "on_cpu_ms"), "ms"),
      formatMetric(schedulerMetric(thread, "runqueue_wait_ms"), "ms"),
      formatMetric(schedulerMetric(thread, "wakeup_latency_avg_us"), "µs"),
      formatMetric(delayMS(thread, "block_io"), "ms", 2),
      formatMetric(metricOrNull(thread.quality?.tracked_ms), "ms"),
    ];

    values.forEach((value, index) => {
      const cell = createElement("td", value === "—" ? "metric-unavailable" : "", String(value));
      if (index === 0) cell.title = String(value);
      row.append(cell);
    });
    body.append(row);
  });
}

function initializeViewer() {
  const state = { reports: [], index: 0, query: "", sort: "cpu" };
  const fileInput = $("#report-file");
  const dropZone = $("#drop-zone");
  const dashboard = $("#report-dashboard");
  const range = $("#interval-range");
  const message = $("#viewer-message");
  const viewer = $("#report-viewer");

  if (!fileInput || !dropZone || !dashboard) return;

  function setMessage(text) {
    message.textContent = text || "";
  }

  function render() {
    const report = state.reports[state.index];
    if (!report) return;

    const visibleThreads = getVisibleThreads(report, state.query, state.sort);
    $("#interval-position").textContent = `${state.index + 1} / ${state.reports.length}`;
    $("#interval-time").textContent = formatTime(report.timestamp || report.interval_end);
    renderQuality(report);
    renderThreadBars(visibleThreads, report);
    renderDelaySummary(visibleThreads);
    renderThreadTable(visibleThreads);
  }

  function loadReports(reports, sourceName) {
    state.reports = reports;
    state.index = 0;
    state.query = "";
    state.sort = "cpu";
    $("#thread-search").value = "";
    $("#thread-sort").value = "cpu";

    const threadIdentities = new Set();
    reports.forEach((report) => report.threads.forEach((thread) => {
      threadIdentities.add(`${thread.tid ?? "?"}:${thread.start_time_ticks ?? thread.start_time_ns ?? "?"}`);
    }));

    $("#summary-pid").textContent = reports[0].pid ?? "—";
    const backends = [...new Set(reports.map((report) => report.backend).filter(Boolean))];
    $("#summary-backend").textContent = backends.join(" / ") || "—";
    $("#summary-intervals").textContent = String(reports.length);
    $("#summary-threads").textContent = String(threadIdentities.size);

    range.min = "0";
    range.max = String(Math.max(0, reports.length - 1));
    range.value = "0";
    range.disabled = reports.length < 2;

    dropZone.hidden = true;
    dashboard.hidden = false;
    setMessage("");
    render();
    dashboard.scrollIntoView({ behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth", block: "nearest" });
    message.textContent = `${sourceName}: ${reports.length} ${reports.length === 1 ? "interval" : "intervals"} loaded.`;
    window.setTimeout(() => {
      if (message.textContent.startsWith(sourceName)) setMessage("");
    }, 3500);
  }

  async function loadFile(file) {
    if (!file) return;
    if (file.size > MAX_REPORT_BYTES) {
      setMessage("This report is larger than 20 MB. Capture fewer intervals or split the file before opening it.");
      return;
    }

    setMessage("Reading the report…");
    try {
      const reports = parseReport(await file.text());
      loadReports(reports, file.name || "Report");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "The report could not be read.");
    } finally {
      fileInput.value = "";
    }
  }

  fileInput.addEventListener("change", () => loadFile(fileInput.files?.[0]));
  dropZone.addEventListener("click", () => fileInput.click());
  dropZone.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      fileInput.click();
    }
  });

  ["dragenter", "dragover"].forEach((eventName) => {
    viewer.addEventListener(eventName, (event) => {
      event.preventDefault();
      viewer.classList.add("is-dragging");
    });
  });
  ["dragleave", "drop"].forEach((eventName) => {
    viewer.addEventListener(eventName, (event) => {
      event.preventDefault();
      viewer.classList.remove("is-dragging");
    });
  });
  viewer.addEventListener("drop", (event) => loadFile(event.dataTransfer?.files?.[0]));

  $("#load-demo").addEventListener("click", async () => {
    setMessage("Loading the demo report…");
    try {
      const response = await fetch("./sample-report.jsonl");
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      loadReports(parseReport(await response.text()), "Demo report");
    } catch (_) {
      setMessage("The demo report is unavailable. You can still open a local tsastat JSONL report.");
    }
  });

  range.addEventListener("input", () => {
    state.index = numeric(range.value);
    render();
  });
  $("#thread-search").addEventListener("input", (event) => {
    state.query = event.target.value;
    render();
  });
  $("#thread-sort").addEventListener("change", (event) => {
    state.sort = event.target.value;
    render();
  });
}

if (typeof document !== "undefined") {
  initializeTheme();
  initializeCopyButtons();
  initializeViewer();
}

if (typeof module !== "undefined" && module.exports) {
  module.exports = { delayMS, getVisibleThreads, parseReport, qualityWarnings };
}

package output

import (
	"encoding/json"
	"io"
	"time"

	"github.com/BogdanDolia/tsastat/internal/model"
)

type JSONRenderer struct {
	encoder        *json.Encoder
	pid            int
	backend        string
	interval       time.Duration
	sampleInterval time.Duration
}

func NewJSONRenderer(w io.Writer, pid int, backendName string, interval, sampleInterval time.Duration) *JSONRenderer {
	enc := json.NewEncoder(w)
	return &JSONRenderer{
		encoder:        enc,
		pid:            pid,
		backend:        backendName,
		interval:       interval,
		sampleInterval: sampleInterval,
	}
}

func (r *JSONRenderer) Render(stats []model.ThreadIntervalStats) error {
	event := jsonInterval{
		Timestamp:        intervalTimestamp(stats).UTC().Format(time.RFC3339Nano),
		PID:              r.pid,
		Backend:          r.backend,
		IntervalMS:       r.interval.Milliseconds(),
		SampleIntervalMS: r.sampleInterval.Milliseconds(),
		Threads:          make([]jsonThread, 0, len(stats)),
	}

	for _, stat := range stats {
		event.Threads = append(event.Threads, jsonThread{
			TID:         stat.TID,
			Comm:        stat.Comm,
			DurationsMS: durationsMS(stat),
			Percent:     percents(stat),
		})
	}

	return r.encoder.Encode(event)
}

type jsonInterval struct {
	Timestamp        string       `json:"timestamp"`
	PID              int          `json:"pid"`
	Backend          string       `json:"backend"`
	IntervalMS       int64        `json:"interval_ms"`
	SampleIntervalMS int64        `json:"sample_interval_ms"`
	Threads          []jsonThread `json:"threads"`
}

type jsonThread struct {
	TID         int                `json:"tid"`
	Comm        string             `json:"comm"`
	DurationsMS map[string]int64   `json:"durations_ms"`
	Percent     map[string]float64 `json:"percent"`
}

func durationsMS(stat model.ThreadIntervalStats) map[string]int64 {
	return map[string]int64{
		string(model.StateRunning):         stat.Duration(model.StateRunning).Milliseconds(),
		string(model.StateSleeping):        stat.Duration(model.StateSleeping).Milliseconds(),
		string(model.StateUninterruptible): stat.Duration(model.StateUninterruptible).Milliseconds(),
		string(model.StateStopped):         stat.Duration(model.StateStopped).Milliseconds(),
		string(model.StateTracingStop):     stat.Duration(model.StateTracingStop).Milliseconds(),
		string(model.StateZombie):          stat.Duration(model.StateZombie).Milliseconds(),
	}
}

func percents(stat model.ThreadIntervalStats) map[string]float64 {
	return map[string]float64{
		string(model.StateRunning):         stat.Percent(model.StateRunning),
		string(model.StateSleeping):        stat.Percent(model.StateSleeping),
		string(model.StateUninterruptible): stat.Percent(model.StateUninterruptible),
	}
}

func intervalTimestamp(stats []model.ThreadIntervalStats) time.Time {
	for _, stat := range stats {
		if !stat.IntervalEnd.IsZero() {
			return stat.IntervalEnd
		}
	}
	return time.Now()
}

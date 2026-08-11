//go:build linux

package ebpf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	ciliumebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"

	"github.com/BogdanDolia/tsastat/internal/backend/proc"
	"github.com/BogdanDolia/tsastat/internal/model"
)

type eventStream struct {
	initial                model.ThreadSnapshot
	events                 chan model.SchedulerEvent
	errors                 chan error
	done                   chan struct{}
	flushed                chan struct{}
	reader                 *ringbuf.Reader
	objects                schedulerObjects
	links                  []link.Link
	monotonicNS            uint64
	wallBase               time.Time
	calibrationUncertainty time.Duration
	lastLost               uint64
	closeOnce              sync.Once
	closeErr               error
}

func (b *Backend) OpenSchedulerEvents(ctx context.Context, pid int) (model.SchedulerEventStream, error) {
	if pid <= 0 {
		return nil, fmt.Errorf("invalid pid %d", pid)
	}

	memlockErr := rlimit.RemoveMemlock()
	stream := &eventStream{
		events:  make(chan model.SchedulerEvent, 4096),
		errors:  make(chan error, 1),
		done:    make(chan struct{}),
		flushed: make(chan struct{}, 1),
	}
	if err := loadSchedulerObjects(&stream.objects, nil); err != nil {
		if memlockErr != nil {
			return nil, fmt.Errorf("load scheduler BPF objects: %w (memlock adjustment also failed: %v)", err, memlockErr)
		}
		return nil, fmt.Errorf("load scheduler BPF objects: %w", err)
	}

	key := uint32(0)
	target := uint32(pid)
	if err := stream.objects.Config.Put(key, target); err != nil {
		stream.Close()
		return nil, fmt.Errorf("configure target pid %d: %w", pid, err)
	}

	reader, err := ringbuf.NewReader(stream.objects.Events)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("open scheduler event ring buffer: %w", err)
	}
	stream.reader = reader

	programs := []struct {
		name    string
		program *ciliumebpf.Program
	}{
		{name: "sched_switch", program: stream.objects.HandleSchedSwitch},
		{name: "sched_wakeup", program: stream.objects.HandleSchedWakeup},
		{name: "sched_wakeup_new", program: stream.objects.HandleSchedWakeupNew},
	}
	for _, program := range programs {
		attached, err := link.AttachTracing(link.TracingOptions{Program: program.program})
		if err != nil {
			stream.Close()
			return nil, fmt.Errorf("attach tp_btf/%s: %w", program.name, err)
		}
		stream.links = append(stream.links, attached)
	}

	stream.wallBase, stream.monotonicNS, stream.calibrationUncertainty, err = clockCalibration()
	if err != nil {
		stream.Close()
		return nil, err
	}
	initial, err := proc.New().Snapshot(ctx, pid)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("initial proc snapshot after eBPF attach: %w", err)
	}
	stream.initial = initial

	go stream.readEvents()
	return stream, nil
}

func (s *eventStream) InitialSnapshot() model.ThreadSnapshot {
	return s.initial
}

func (s *eventStream) Events() <-chan model.SchedulerEvent {
	return s.events
}

func (s *eventStream) Errors() <-chan error {
	return s.errors
}

func (s *eventStream) Flush() error {
	if err := s.reader.Flush(); err != nil {
		return fmt.Errorf("flush scheduler event ring buffer: %w", err)
	}
	return nil
}

func (s *eventStream) Flushed() <-chan struct{} {
	return s.flushed
}

func (s *eventStream) LostEvents() uint64 {
	key := uint32(0)
	var value uint64
	if s.objects.LostEvents != nil && s.objects.LostEvents.Lookup(key, &value) == nil {
		s.lastLost = value
	}
	return s.lastLost
}

func (s *eventStream) ClockCalibrationUncertainty() time.Duration {
	return s.calibrationUncertainty
}

func (s *eventStream) Close() error {
	s.closeOnce.Do(func() {
		var closeErrors []error
		close(s.done)
		if s.reader != nil {
			closeErrors = append(closeErrors, s.reader.Close())
		}
		for _, attached := range s.links {
			closeErrors = append(closeErrors, attached.Close())
		}
		closeErrors = append(closeErrors, s.objects.Close())
		s.closeErr = errors.Join(closeErrors...)
	})
	return s.closeErr
}

func (s *eventStream) readEvents() {
	defer close(s.events)
	defer close(s.errors)
	defer close(s.flushed)
	for {
		record, err := s.reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrFlushed) {
				select {
				case s.flushed <- struct{}{}:
				case <-s.done:
					return
				}
				continue
			}
			if !errors.Is(err, ringbuf.ErrClosed) {
				select {
				case s.errors <- fmt.Errorf("read scheduler event ring buffer: %w", err):
				case <-s.done:
				}
			}
			return
		}
		event, err := s.decodeEvent(record.RawSample)
		if err != nil {
			select {
			case s.errors <- err:
			case <-s.done:
			}
			return
		}
		select {
		case s.events <- event:
		case <-s.done:
			return
		}
	}
}

func (s *eventStream) decodeEvent(raw []byte) (model.SchedulerEvent, error) {
	timestampNS, event, err := decodeSchedulerEvent(raw)
	if err != nil {
		return model.SchedulerEvent{}, err
	}
	event.Timestamp = s.kernelTime(timestampNS)
	return event, nil
}

func (s *eventStream) kernelTime(timestampNS uint64) time.Time {
	if timestampNS >= s.monotonicNS {
		return s.wallBase.Add(time.Duration(timestampNS - s.monotonicNS))
	}
	return s.wallBase.Add(-time.Duration(s.monotonicNS - timestampNS))
}

func clockCalibration() (time.Time, uint64, time.Duration, error) {
	before := time.Now()
	var monotonic unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &monotonic); err != nil {
		return time.Time{}, 0, 0, fmt.Errorf("read CLOCK_MONOTONIC: %w", err)
	}
	after := time.Now()
	uncertainty := after.Sub(before) / 2
	return before.Add(uncertainty), uint64(monotonic.Nano()), uncertainty, nil
}

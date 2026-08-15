package diagnostics

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	runtimemetrics "runtime/metrics"
	"runtime/pprof"
	"sort"
	"sync/atomic"
	"time"
)

type pointStats struct {
	calls      atomic.Uint64
	errors     atomic.Uint64
	samples    atomic.Uint64
	totalNanos atomic.Uint64
	minNanos   atomic.Uint64
	maxNanos   atomic.Uint64
	allocs     atomic.Uint64
	allocBytes atomic.Uint64
	frees      atomic.Uint64
}

type metricsObservation struct {
	started                    time.Time
	mallocs, totalAlloc, frees uint64
	timing, allocs             bool
}

type MetricsSnapshot struct {
	Calls      uint64 `json:"calls"`
	Errors     uint64 `json:"errors"`
	TotalNanos uint64 `json:"total_nanos"`
	MinNanos   uint64 `json:"min_nanos"`
	MaxNanos   uint64 `json:"max_nanos"`
	AvgNanos   uint64 `json:"avg_nanos"`
	Allocs     uint64 `json:"allocs"`
	AllocBytes uint64 `json:"alloc_bytes"`
	Frees      uint64 `json:"frees"`
}

type RuntimeSnapshot struct {
	Timestamp     time.Time `json:"timestamp"`
	HeapAlloc     uint64    `json:"heap_alloc"`
	HeapObjects   uint64    `json:"heap_objects"`
	HeapInuse     uint64    `json:"heap_inuse"`
	HeapIdle      uint64    `json:"heap_idle"`
	HeapReleased  uint64    `json:"heap_released"`
	NextGC        uint64    `json:"next_gc"`
	NumGC         uint32    `json:"num_gc"`
	PauseTotalNS  uint64    `json:"pause_total_ns"`
	GCCPUFraction float64  `json:"gc_cpu_fraction"`
	LiveHeap      uint64    `json:"live_heap"`
	HeapGoal      uint64    `json:"heap_goal"`
	GCCycles      uint64    `json:"gc_cycles"`
	Goroutines    uint64    `json:"goroutines"`
}

type runtimeLine struct {
	Type string `json:"type"`
	RuntimeSnapshot
}

var runtimeSnapshotReader = captureRuntimeSnapshot
var forceGarbageCollection = runtime.GC
var heapProfileWriter = pprof.WriteHeapProfile

func beginMetricsObservation(rule *compiledMetricsRule) metricsObservation {
	observation := metricsObservation{timing: rule.captureTiming, allocs: rule.captureAllocs}
	if observation.timing {
		observation.started = time.Now()
	}
	if observation.allocs {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		observation.mallocs = stats.Mallocs
		observation.totalAlloc = stats.TotalAlloc
		observation.frees = stats.Frees
	}
	return observation
}

func endMetricsObservation(rule *compiledMetricsRule, observation metricsObservation, err error) {
	stats := &rule.stats
	if rule.captureCalls {
		stats.calls.Add(1)
	}
	if rule.captureErrors && err != nil {
		stats.errors.Add(1)
	}
	if observation.timing {
		nanos := uint64(time.Since(observation.started))
		stats.samples.Add(1)
		stats.totalNanos.Add(nanos)
		for {
			current := stats.minNanos.Load()
			if nanos >= current || stats.minNanos.CompareAndSwap(current, nanos) {
				break
			}
		}
		for {
			current := stats.maxNanos.Load()
			if nanos <= current || stats.maxNanos.CompareAndSwap(current, nanos) {
				break
			}
		}
	}
	if observation.allocs {
		var current runtime.MemStats
		runtime.ReadMemStats(&current)
		stats.allocs.Add(current.Mallocs - observation.mallocs)
		stats.allocBytes.Add(current.TotalAlloc - observation.totalAlloc)
		stats.frees.Add(current.Frees - observation.frees)
	}
}

func snapshot(rule *compiledMetricsRule) MetricsSnapshot {
	stats := &rule.stats
	samples := stats.samples.Load()
	min := stats.minNanos.Load()
	if samples == 0 || min == math.MaxUint64 {
		min = 0
	}
	total := stats.totalNanos.Load()
	average := uint64(0)
	if samples > 0 {
		average = total / samples
	}
	return MetricsSnapshot{
		Calls: stats.calls.Load(), Errors: stats.errors.Load(), TotalNanos: total, MinNanos: min, MaxNanos: stats.maxNanos.Load(), AvgNanos: average,
		Allocs: stats.allocs.Load(), AllocBytes: stats.allocBytes.Load(), Frees: stats.frees.Load(),
	}
}

func emitMetrics(output io.Writer, metrics []*compiledMetricsRule) error {
	metrics = append([]*compiledMetricsRule(nil), metrics...)
	sort.Slice(metrics, func(left, right int) bool { return metrics[left].id < metrics[right].id })
	for _, metric := range metrics {
		line, err := json.Marshal(metricsLine{RuleID: metric.id, Point: metric.op, SampleEvery: metric.sampleEvery, MetricsSnapshot: snapshot(metric)})
		if err != nil {
			return err
		}
		if _, err := output.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return nil
}

type metricsLine struct {
	RuleID      string `json:"rule_id"`
	Point       string `json:"point"`
	SampleEvery uint64 `json:"sample_every"`
	MetricsSnapshot
}

func EmitRuntime(output io.Writer) error {
	line, err := json.Marshal(runtimeLine{Type: "runtime", RuntimeSnapshot: runtimeSnapshotReader()})
	if err != nil {
		return err
	}
	_, err = output.Write(append(line, '\n'))
	return err
}

func captureRuntimeSnapshot() RuntimeSnapshot {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	samples := []runtimemetrics.Sample{
		{Name: "/gc/heap/live:bytes"},
		{Name: "/gc/heap/goal:bytes"},
		{Name: "/gc/cycles/total:gc-cycles"},
		{Name: "/sched/goroutines:goroutines"},
	}
	runtimemetrics.Read(samples)
	return RuntimeSnapshot{
		Timestamp: time.Now().UTC(), HeapAlloc: stats.HeapAlloc, HeapObjects: stats.HeapObjects, HeapInuse: stats.HeapInuse,
		HeapIdle: stats.HeapIdle, HeapReleased: stats.HeapReleased, NextGC: stats.NextGC, NumGC: stats.NumGC,
		PauseTotalNS: stats.PauseTotalNs, GCCPUFraction: stats.GCCPUFraction, LiveHeap: metricUint64(samples[0]),
		HeapGoal: metricUint64(samples[1]), GCCycles: metricUint64(samples[2]), Goroutines: metricUint64(samples[3]),
	}
}

func metricUint64(sample runtimemetrics.Sample) uint64 {
	if sample.Value.Kind() != runtimemetrics.KindUint64 {
		return 0
	}
	return sample.Value.Uint64()
}

func writeHeapProfileToDirectory(directory string, forceGC bool) error {
	if forceGC {
		forceGarbageCollection()
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create diagnostics directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("restrict diagnostics directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".heap-*.tmp")
	if err != nil {
		return fmt.Errorf("create heap profile: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("restrict heap profile: %w", err)
	}
	if err := heapProfileWriter(temporary); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write heap profile: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync heap profile: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close heap profile: %w", err)
	}
	path := filepath.Join(directory, fmt.Sprintf("heap-%d-%d.pprof", time.Now().UTC().UnixNano(), os.Getpid()))
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish heap profile: %w", err)
	}
	return nil
}

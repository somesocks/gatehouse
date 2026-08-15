package diagnostics

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

var allocationSink []byte

func init() {
	Register("diagnostics.test")
}

func boolPointer(value bool) *bool { return &value }

func metricRule(id string, capture MetricsCaptureConfig) RuleConfig {
	return RuleConfig{
		ID: id, Op: "diagnostics.test", Key: "*", When: WhenConfig{Mode: "every_x", X: 1},
		Action: ActionConfig{Type: "metrics", Capture: capture},
	}
}

func setup(t *testing.T, config Config) {
	t.Helper()
	Disable()
	t.Cleanup(Disable)
	if err := SetupFromConfig(config); err != nil {
		t.Fatal(err)
	}
}

func TestParseV2YAMLFile(t *testing.T) {
	path := t.TempDir() + "/diagnostics.yaml"
	contents := []byte("version: 2\noutputs:\n  stderr:\n    type: stderr\ntriggers:\n  - id: exit\n    source:\n      type: process_exit\n    actions:\n      - type: emit_runtime\n        output: stderr\n")
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := parseConfigFromEnv("file:" + path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Version != 2 || config.Outputs["stderr"].Type != "stderr" || len(config.Triggers) != 1 {
		t.Fatalf("config = %+v", config)
	}
}

func TestMetricsCaptureAllocs(t *testing.T) {
	setup(t, Config{Version: 2, Rules: []RuleConfig{metricRule("allocs", MetricsCaptureConfig{Calls: boolPointer(true), Errors: boolPointer(true), Timing: boolPointer(true), Allocs: boolPointer(true)})}})
	call, err := Begin("diagnostics.test", "test")
	if err != nil {
		t.Fatal(err)
	}
	allocationSink = make([]byte, 4096)
	if err := call.End(nil); err != nil {
		t.Fatal(err)
	}
	metrics := activeEngine.Load().metrics["allocs"]
	snapshot := snapshot(metrics)
	if snapshot.Calls != 1 || snapshot.Allocs == 0 || snapshot.AllocBytes < 4096 {
		t.Fatalf("metrics = %+v", snapshot)
	}
}

func TestMetricsCapturesErrors(t *testing.T) {
	setup(t, Config{Version: 2, Rules: []RuleConfig{metricRule("errors", MetricsCaptureConfig{})}})
	call, err := Begin("diagnostics.test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := call.End(errors.New("failed")); err == nil {
		t.Fatal("End() returned nil")
	}
	snapshot := snapshot(activeEngine.Load().metrics["errors"])
	if snapshot.Calls != 1 || snapshot.Errors != 1 {
		t.Fatalf("metrics = %+v", snapshot)
	}
}

func TestErrorActionInjectsConfiguredError(t *testing.T) {
	setup(t, Config{Version: 2, Rules: []RuleConfig{{
		ID: "inject", Op: "diagnostics.test", Key: "*", When: WhenConfig{Mode: "before_x", X: 1}, Action: ActionConfig{Type: "error", Error: "EIO"},
	}}})
	_, err := Begin("diagnostics.test", "test")
	if !errors.Is(err, syscall.EIO) {
		t.Fatalf("Begin() error = %v", err)
	}
}

func TestSignalEmitsConfiguredMetrics(t *testing.T) {
	setup(t, Config{
		Version: 2,
		Outputs: map[string]OutputConfig{"stderr": {Type: "stderr"}},
		Rules:   []RuleConfig{metricRule("metric", MetricsCaptureConfig{})},
		Triggers: []TriggerConfig{{
			ID: "metric-signal", Source: TriggerSourceConfig{Type: "signal", Signal: "USR1"},
			Actions: []TriggerActionConfig{{Type: "emit_metrics", Output: "stderr", Rules: []string{"metric"}}},
		}},
	})
	call, err := Begin("diagnostics.test", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := call.End(nil); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := EmitSignal(syscall.SIGUSR1, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), `"rule_id":"metric"`) {
		t.Fatalf("signal output = %q", stderr.String())
	}
}

func TestSignalEmitsRuntime(t *testing.T) {
	original := runtimeSnapshotReader
	runtimeSnapshotReader = func() RuntimeSnapshot {
		return RuntimeSnapshot{Timestamp: time.Unix(1, 0).UTC(), HeapAlloc: 123, Goroutines: 4}
	}
	t.Cleanup(func() { runtimeSnapshotReader = original })
	setup(t, Config{
		Version: 2,
		Outputs: map[string]OutputConfig{"stderr": {Type: "stderr"}},
		Triggers: []TriggerConfig{{
			ID: "runtime-signal", Source: TriggerSourceConfig{Type: "signal", Signal: "USR1"},
			Actions: []TriggerActionConfig{{Type: "emit_runtime", Output: "stderr"}},
		}},
	})
	var stdout, stderr bytes.Buffer
	if err := EmitSignal(syscall.SIGUSR1, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var line runtimeLine
	if err := json.Unmarshal(stderr.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	if line.Type != "runtime" || line.HeapAlloc != 123 || line.Goroutines != 4 {
		t.Fatalf("runtime output = %+v", line)
	}
}

func TestHeapProfileForceGCBeforeWrite(t *testing.T) {
	originalGC := forceGarbageCollection
	originalWriter := heapProfileWriter
	var calls []string
	forceGarbageCollection = func() { calls = append(calls, "gc") }
	heapProfileWriter = func(writer io.Writer) error {
		calls = append(calls, "write")
		_, err := writer.Write([]byte("profile"))
		return err
	}
	t.Cleanup(func() {
		forceGarbageCollection = originalGC
		heapProfileWriter = originalWriter
	})
	directory := t.TempDir()
	setup(t, Config{
		Version: 2,
		Outputs: map[string]OutputConfig{"profiles": {Type: "directory", Path: directory}},
		Triggers: []TriggerConfig{{
			ID: "heap-signal", Source: TriggerSourceConfig{Type: "signal", Signal: "USR2"},
			Actions: []TriggerActionConfig{{Type: "write_heap_profile", Output: "profiles", GC: "force"}},
		}},
	})
	if err := EmitSignal(syscall.SIGUSR2, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "gc,write" {
		t.Fatalf("heap profile actions = %v", calls)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("heap profiles = %v", entries)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("heap profile info = %v", info)
	}
}

func TestHeapProfileWritesProfile(t *testing.T) {
	directory := t.TempDir()
	setup(t, Config{
		Version: 2,
		Outputs: map[string]OutputConfig{"profiles": {Type: "directory", Path: directory}},
		Triggers: []TriggerConfig{{
			ID: "heap-signal", Source: TriggerSourceConfig{Type: "signal", Signal: "USR2"},
			Actions: []TriggerActionConfig{{Type: "write_heap_profile", Output: "profiles"}},
		}},
	})
	if err := EmitSignal(syscall.SIGUSR2, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("heap profiles = %v", entries)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 || info.Mode().Perm() != 0o600 {
		t.Fatalf("heap profile info = %v", info)
	}
}

func TestProcessExitEmitsConfiguredMetrics(t *testing.T) {
	setup(t, Config{
		Version: 2,
		Outputs: map[string]OutputConfig{"stderr": {Type: "stderr"}},
		Rules:   []RuleConfig{metricRule("metric", MetricsCaptureConfig{})},
		Triggers: []TriggerConfig{{
			ID: "exit", Source: TriggerSourceConfig{Type: "process_exit"},
			Actions: []TriggerActionConfig{{Type: "emit_metrics", Output: "stderr"}},
		}},
	})
	var stdout, stderr bytes.Buffer
	if err := EmitProcessExit(&stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), `"rule_id":"metric"`) {
		t.Fatalf("exit output = %q", stderr.String())
	}
}

func TestTriggerRejectsInvalidOutput(t *testing.T) {
	err := SetupFromConfig(Config{
		Version: 2,
		Outputs: map[string]OutputConfig{"stderr": {Type: "stderr"}},
		Triggers: []TriggerConfig{{
			ID: "invalid", Source: TriggerSourceConfig{Type: "signal", Signal: "USR1"},
			Actions: []TriggerActionConfig{{Type: "write_heap_profile", Output: "stderr"}},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "requires a directory output") {
		t.Fatalf("SetupFromConfig() error = %v", err)
	}
}

func TestBeginEndAllocateNothingWhenDisabled(t *testing.T) {
	Disable()
	t.Cleanup(Disable)
	allocations := testing.AllocsPerRun(100, func() {
		call, err := Begin("diagnostics.test", "test")
		if err != nil {
			t.Fatal(err)
		}
		if err := call.End(nil); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("disabled allocations = %f", allocations)
	}
}

func TestBeginEndAllocateNothingForMetrics(t *testing.T) {
	setup(t, Config{Version: 2, Rules: []RuleConfig{metricRule("metric", MetricsCaptureConfig{Calls: boolPointer(true), Errors: boolPointer(true), Timing: boolPointer(true)})}})
	allocations := testing.AllocsPerRun(100, func() {
		call, err := Begin("diagnostics.test", "test")
		if err != nil {
			t.Fatal(err)
		}
		if err := call.End(nil); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("metrics allocations = %f", allocations)
	}
}

func TestBeginEndAllocateNothingForAllocs(t *testing.T) {
	setup(t, Config{Version: 2, Rules: []RuleConfig{metricRule("metric", MetricsCaptureConfig{Allocs: boolPointer(true)})}})
	allocations := testing.AllocsPerRun(100, func() {
		call, err := Begin("diagnostics.test", "test")
		if err != nil {
			t.Fatal(err)
		}
		if err := call.End(nil); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("alloc metrics allocations = %f", allocations)
	}
}

func BenchmarkMetrics(b *testing.B) {
	Disable()
	if err := SetupFromConfig(Config{Version: 2, Rules: []RuleConfig{metricRule("metric", MetricsCaptureConfig{})}}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(Disable)
	b.ReportAllocs()
	for b.Loop() {
		call, err := Begin("diagnostics.test", "test")
		if err != nil {
			b.Fatal(err)
		}
		if err := call.End(nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMetricsAllocs(b *testing.B) {
	Disable()
	if err := SetupFromConfig(Config{Version: 2, Rules: []RuleConfig{metricRule("metric", MetricsCaptureConfig{Allocs: boolPointer(true)})}}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(Disable)
	b.ReportAllocs()
	for b.Loop() {
		call, err := Begin("diagnostics.test", "test")
		if err != nil {
			b.Fatal(err)
		}
		if err := call.End(nil); err != nil {
			b.Fatal(err)
		}
	}
}

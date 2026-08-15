package diagnostics

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type engine struct {
	rules       map[string][]*compiledRule
	metrics     map[string]*compiledMetricsRule
	signals     map[syscall.Signal][]compiledTriggerAction
	exitActions []compiledTriggerAction
	triggerMu   sync.Mutex
}

type matcherKind uint8

const (
	matcherAny matcherKind = iota
	matcherExact
	matcherPrefix
)

type matcher struct {
	kind  matcherKind
	value string
}

func (matcher matcher) matches(key string) bool {
	switch matcher.kind {
	case matcherAny:
		return true
	case matcherExact:
		return key == matcher.value
	case matcherPrefix:
		return strings.HasPrefix(key, matcher.value)
	default:
		return false
	}
}

type whenMode uint8

const (
	whenBefore whenMode = iota
	whenAfter
	whenEvery
)

type actionType uint8

const (
	actionError actionType = iota
	actionDelay
	actionMetrics
)

type outputKind uint8

const (
	outputStderr outputKind = iota
	outputStdout
	outputDirectory
)

type compiledOutput struct {
	kind outputKind
	path string
}

type triggerActionType uint8

const (
	triggerEmitMetrics triggerActionType = iota
	triggerEmitRuntime
	triggerWriteHeapProfile
)

type compiledTriggerAction struct {
	typeID  triggerActionType
	metrics []*compiledMetricsRule
	output  compiledOutput
	forceGC bool
}

type compiledMetricsRule struct {
	id            string
	op            string
	captureCalls  bool
	captureErrors bool
	captureTiming bool
	captureAllocs bool
	sampleEvery   uint64
	stats         pointStats
}

type compiledRule struct {
	id        string
	matcher   matcher
	when      whenMode
	n         uint64
	limit     int64
	counter   atomic.Uint64
	hits      atomic.Int64
	action    actionType
	postError bool
	delay     time.Duration
	err       error
	metric    *compiledMetricsRule
}

var timeSleep = time.Sleep

func compileConfig(config Config) (*engine, error) {
	if config.Version != 2 {
		return nil, fmt.Errorf("unsupported diagnostics version %d", config.Version)
	}
	outputs, err := compileOutputs(config.Outputs)
	if err != nil {
		return nil, err
	}
	compiled := &engine{
		rules:   map[string][]*compiledRule{},
		metrics: map[string]*compiledMetricsRule{},
		signals: map[syscall.Signal][]compiledTriggerAction{},
	}
	ids := map[string]struct{}{}
	for index, ruleConfig := range config.Rules {
		if ruleConfig.Enabled != nil && !*ruleConfig.Enabled {
			continue
		}
		rule, point, err := compileRule(index, ruleConfig)
		if err != nil {
			return nil, err
		}
		if _, found := ids[rule.id]; found {
			return nil, fmt.Errorf("diagnostics rule %q is duplicated", rule.id)
		}
		ids[rule.id] = struct{}{}
		if !isRegistered(point) {
			return nil, fmt.Errorf("diagnostics rule %q uses unknown operation %q", rule.id, point)
		}
		compiled.rules[point] = append(compiled.rules[point], rule)
		if len(compiled.rules[point]) > maxMatchedRules {
			return nil, fmt.Errorf("diagnostics operation %q has more than %d rules", point, maxMatchedRules)
		}
		if rule.metric != nil {
			compiled.metrics[rule.metric.id] = rule.metric
		}
	}
	if err := compileTriggers(compiled, outputs, config.Triggers); err != nil {
		return nil, err
	}
	return compiled, nil
}

func compileOutputs(configs map[string]OutputConfig) (map[string]compiledOutput, error) {
	outputs := make(map[string]compiledOutput, len(configs))
	for rawName, config := range configs {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, fmt.Errorf("diagnostics output has an empty name")
		}
		if _, found := outputs[name]; found {
			return nil, fmt.Errorf("diagnostics output %q is duplicated", name)
		}
		output, err := compileOutput(name, config)
		if err != nil {
			return nil, err
		}
		outputs[name] = output
	}
	return outputs, nil
}

func compileOutput(name string, config OutputConfig) (compiledOutput, error) {
	switch strings.TrimSpace(config.Type) {
	case "stderr":
		if config.Path != "" {
			return compiledOutput{}, fmt.Errorf("diagnostics output %q: stderr does not accept path", name)
		}
		return compiledOutput{kind: outputStderr}, nil
	case "stdout":
		if config.Path != "" {
			return compiledOutput{}, fmt.Errorf("diagnostics output %q: stdout does not accept path", name)
		}
		return compiledOutput{kind: outputStdout}, nil
	case "directory":
		path := filepath.Clean(strings.TrimSpace(config.Path))
		if config.Path == "" || !filepath.IsAbs(path) {
			return compiledOutput{}, fmt.Errorf("diagnostics output %q: directory path must be absolute", name)
		}
		return compiledOutput{kind: outputDirectory, path: path}, nil
	default:
		return compiledOutput{}, fmt.Errorf("diagnostics output %q: unsupported type %q", name, config.Type)
	}
}

func compileRule(index int, config RuleConfig) (*compiledRule, string, error) {
	id := strings.TrimSpace(config.ID)
	if id == "" {
		id = fmt.Sprintf("rule-%d", index+1)
	}
	point := strings.TrimSpace(config.Op)
	if point == "" {
		return nil, "", fmt.Errorf("diagnostics rule %q: missing op", id)
	}
	matched, err := compileMatcher(config.Key)
	if err != nil {
		return nil, "", fmt.Errorf("diagnostics rule %q: %w", id, err)
	}
	when, n, err := compileWhen(config.When)
	if err != nil {
		return nil, "", fmt.Errorf("diagnostics rule %q: %w", id, err)
	}
	if config.When.Limit < 0 {
		return nil, "", fmt.Errorf("diagnostics rule %q: when.limit must not be negative", id)
	}
	rule := &compiledRule{id: id, matcher: matched, when: when, n: n, limit: config.When.Limit}
	if err := compileAction(rule, config.Action); err != nil {
		return nil, "", fmt.Errorf("diagnostics rule %q: %w", id, err)
	}
	if rule.metric != nil {
		rule.metric.op = point
		if when != whenEvery {
			rule.metric.sampleEvery = 1
		}
	}
	return rule, point, nil
}

func compileMatcher(raw string) (matcher, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "*":
		return matcher{kind: matcherAny}, nil
	case strings.HasPrefix(raw, "prefix:"):
		value := strings.TrimPrefix(raw, "prefix:")
		if value == "" {
			return matcher{}, fmt.Errorf("key prefix is empty")
		}
		return matcher{kind: matcherPrefix, value: value}, nil
	case raw != "":
		return matcher{kind: matcherExact, value: raw}, nil
	default:
		return matcher{}, fmt.Errorf("key is required")
	}
}

func compileWhen(config WhenConfig) (whenMode, uint64, error) {
	if config.X <= 0 {
		return 0, 0, fmt.Errorf("when.x must be positive")
	}
	switch config.Mode {
	case "before_x":
		return whenBefore, uint64(config.X), nil
	case "after_x":
		return whenAfter, uint64(config.X), nil
	case "every_x":
		return whenEvery, uint64(config.X), nil
	default:
		return 0, 0, fmt.Errorf("unsupported when.mode %q", config.Mode)
	}
}

func compileAction(rule *compiledRule, config ActionConfig) error {
	switch config.Type {
	case "delay":
		if config.DelayMS < 0 || config.Phase != "" || config.Error != "" || hasCapture(config.Capture) {
			return fmt.Errorf("invalid delay action")
		}
		rule.action = actionDelay
		rule.delay = time.Duration(config.DelayMS) * time.Millisecond
		return nil
	case "error":
		if config.DelayMS != 0 || hasCapture(config.Capture) {
			return fmt.Errorf("invalid error action")
		}
		configuredErr, err := parseError(config.Error)
		if err != nil {
			return err
		}
		post, err := parsePhase(config.Phase)
		if err != nil {
			return err
		}
		rule.action = actionError
		rule.err = configuredErr
		rule.postError = post
		return nil
	case "metrics":
		if config.DelayMS != 0 || config.Phase != "" || config.Error != "" {
			return fmt.Errorf("invalid metrics action")
		}
		metric := &compiledMetricsRule{
			id:            rule.id,
			captureCalls:  boolOrDefault(config.Capture.Calls, true),
			captureErrors: boolOrDefault(config.Capture.Errors, true),
			captureTiming: boolOrDefault(config.Capture.Timing, true),
			captureAllocs: boolOrDefault(config.Capture.Allocs, false),
			sampleEvery:   rule.n,
		}
		if !metric.captureCalls && !metric.captureErrors && !metric.captureTiming && !metric.captureAllocs {
			return fmt.Errorf("metrics capture must enable at least one value")
		}
		metric.stats.minNanos.Store(math.MaxUint64)
		rule.action = actionMetrics
		rule.metric = metric
		return nil
	default:
		return fmt.Errorf("unsupported action.type %q", config.Type)
	}
}

func compileTriggers(compiled *engine, outputs map[string]compiledOutput, configs []TriggerConfig) error {
	ids := map[string]struct{}{}
	sources := map[string]struct{}{}
	for _, config := range configs {
		id := strings.TrimSpace(config.ID)
		if id == "" {
			return fmt.Errorf("diagnostics trigger has an empty id")
		}
		if _, found := ids[id]; found {
			return fmt.Errorf("diagnostics trigger %q is duplicated", id)
		}
		ids[id] = struct{}{}
		if len(config.Actions) == 0 {
			return fmt.Errorf("diagnostics trigger %q has no actions", id)
		}
		actions := make([]compiledTriggerAction, 0, len(config.Actions))
		for _, actionConfig := range config.Actions {
			action, err := compileTriggerAction(id, compiled.metrics, outputs, actionConfig)
			if err != nil {
				return err
			}
			actions = append(actions, action)
		}
		switch strings.TrimSpace(config.Source.Type) {
		case "signal":
			signal, err := parseSignal(config.Source.Signal)
			if err != nil {
				return fmt.Errorf("diagnostics trigger %q: %w", id, err)
			}
			source := fmt.Sprintf("signal:%d", signal)
			if _, found := sources[source]; found {
				return fmt.Errorf("diagnostics trigger %q duplicates source %q", id, source)
			}
			sources[source] = struct{}{}
			compiled.signals[signal] = actions
		case "process_exit":
			if strings.TrimSpace(config.Source.Signal) != "" {
				return fmt.Errorf("diagnostics trigger %q: process_exit does not accept signal", id)
			}
			if _, found := sources["process_exit"]; found {
				return fmt.Errorf("diagnostics trigger %q duplicates process_exit", id)
			}
			sources["process_exit"] = struct{}{}
			compiled.exitActions = actions
		default:
			return fmt.Errorf("diagnostics trigger %q: unsupported source type %q", id, config.Source.Type)
		}
	}
	return nil
}

func compileTriggerAction(triggerID string, metrics map[string]*compiledMetricsRule, outputs map[string]compiledOutput, config TriggerActionConfig) (compiledTriggerAction, error) {
	outputName := strings.TrimSpace(config.Output)
	output, found := outputs[outputName]
	if !found {
		return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: unknown output %q", triggerID, outputName)
	}
	switch config.Type {
	case "emit_metrics":
		if output.kind == outputDirectory {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: emit_metrics requires stdout or stderr", triggerID)
		}
		if config.GC != "" {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: emit_metrics does not accept gc", triggerID)
		}
		selected, err := selectMetrics(metrics, config.Rules)
		if err != nil {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: %w", triggerID, err)
		}
		return compiledTriggerAction{typeID: triggerEmitMetrics, metrics: selected, output: output}, nil
	case "emit_runtime":
		if output.kind == outputDirectory {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: emit_runtime requires stdout or stderr", triggerID)
		}
		if len(config.Rules) != 0 || config.GC != "" {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: invalid emit_runtime action", triggerID)
		}
		return compiledTriggerAction{typeID: triggerEmitRuntime, output: output}, nil
	case "write_heap_profile":
		if output.kind != outputDirectory {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: write_heap_profile requires a directory output", triggerID)
		}
		if len(config.Rules) != 0 {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: write_heap_profile does not accept rules", triggerID)
		}
		forceGC, err := parseGC(config.GC)
		if err != nil {
			return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: %w", triggerID, err)
		}
		return compiledTriggerAction{typeID: triggerWriteHeapProfile, output: output, forceGC: forceGC}, nil
	default:
		return compiledTriggerAction{}, fmt.Errorf("diagnostics trigger %q: unsupported action %q", triggerID, config.Type)
	}
}

func selectMetrics(metrics map[string]*compiledMetricsRule, ruleIDs []string) ([]*compiledMetricsRule, error) {
	if len(ruleIDs) == 0 {
		selected := make([]*compiledMetricsRule, 0, len(metrics))
		for _, metric := range metrics {
			selected = append(selected, metric)
		}
		return selected, nil
	}
	selected := make([]*compiledMetricsRule, 0, len(ruleIDs))
	seen := map[string]struct{}{}
	for _, rawID := range ruleIDs {
		id := strings.TrimSpace(rawID)
		if _, found := seen[id]; found {
			return nil, fmt.Errorf("metrics rule %q is duplicated", id)
		}
		seen[id] = struct{}{}
		metric := metrics[id]
		if metric == nil {
			return nil, fmt.Errorf("unknown metrics rule %q", id)
		}
		selected = append(selected, metric)
	}
	return selected, nil
}

func parseError(raw string) (error, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "EIO":
		return syscall.EIO, nil
	case "EMLINK":
		return syscall.EMLINK, nil
	case "EXDEV":
		return syscall.EXDEV, nil
	case "ETIMEDOUT":
		return syscall.ETIMEDOUT, nil
	default:
		return nil, fmt.Errorf("unsupported error %q", raw)
	}
}

func parsePhase(raw string) (bool, error) {
	switch strings.TrimSpace(raw) {
	case "", "pre":
		return false, nil
	case "post":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported action.phase %q", raw)
	}
}

func parseSignal(raw string) (syscall.Signal, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "USR1":
		return syscall.SIGUSR1, nil
	case "USR2":
		return syscall.SIGUSR2, nil
	case "HUP":
		return syscall.SIGHUP, nil
	default:
		return 0, fmt.Errorf("unsupported signal %q", raw)
	}
}

func parseGC(raw string) (bool, error) {
	switch strings.TrimSpace(raw) {
	case "", "none":
		return false, nil
	case "force":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported gc mode %q", raw)
	}
}

func boolOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func hasCapture(capture MetricsCaptureConfig) bool {
	return capture.Calls != nil || capture.Errors != nil || capture.Timing != nil || capture.Allocs != nil
}

func (rule *compiledRule) matches(key string) bool {
	if !rule.matcher.matches(key) {
		return false
	}
	count := rule.counter.Add(1)
	matched := false
	switch rule.when {
	case whenBefore:
		matched = count <= rule.n
	case whenAfter:
		matched = count > rule.n
	case whenEvery:
		matched = count%rule.n == 0
	}
	if !matched || rule.limit == 0 {
		return matched && rule.limit == 0
	}
	for {
		current := rule.hits.Load()
		if current >= rule.limit {
			return false
		}
		if rule.hits.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

package diagnostics

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
)

const maxMatchedRules = 16

var activeEngine atomic.Pointer[engine]

var registeredPoints = struct {
	sync.RWMutex
	values map[string]struct{}
}{values: map[string]struct{}{}}

// Register declares an operation point accepted by diagnostics configuration.
// Packages call this from init, before main loads GATEHOUSE_DIAG.
func Register(point string) {
	registeredPoints.Lock()
	registeredPoints.values[point] = struct{}{}
	registeredPoints.Unlock()
}

func Disable() {
	activeEngine.Store(nil)
}

func SetupFromEnv() error {
	raw := os.Getenv(EnvVar)
	if raw == "" {
		Disable()
		return nil
	}
	config, err := parseConfigFromEnv(raw)
	if err != nil {
		return err
	}
	return SetupFromConfig(config)
}

func SetupFromConfig(config Config) error {
	compiled, err := compileConfig(config)
	if err != nil {
		return err
	}
	activeEngine.Store(compiled)
	return nil
}

func isRegistered(point string) bool {
	registeredPoints.RLock()
	_, found := registeredPoints.values[point]
	registeredPoints.RUnlock()
	return found
}

func Signals() []os.Signal {
	current := activeEngine.Load()
	if current == nil || len(current.signals) == 0 {
		return nil
	}
	signals := make([]os.Signal, 0, len(current.signals))
	for signal := range current.signals {
		signals = append(signals, signal)
	}
	return signals
}

func EmitSignal(signal os.Signal, stdout, stderr io.Writer) error {
	value, ok := signal.(syscall.Signal)
	if !ok {
		return nil
	}
	current := activeEngine.Load()
	if current == nil {
		return nil
	}
	return current.emit(current.signals[value], stdout, stderr)
}

func EmitProcessExit(stdout, stderr io.Writer) error {
	current := activeEngine.Load()
	if current == nil {
		return nil
	}
	return current.emit(current.exitActions, stdout, stderr)
}

func (current *engine) emit(actions []compiledTriggerAction, stdout, stderr io.Writer) error {
	if len(actions) == 0 {
		return nil
	}
	current.triggerMu.Lock()
	defer current.triggerMu.Unlock()
	for _, action := range actions {
		switch action.typeID {
		case triggerEmitMetrics:
			if err := emitMetrics(outputWriter(action.output, stdout, stderr), action.metrics); err != nil {
				return err
			}
		case triggerEmitRuntime:
			if err := EmitRuntime(outputWriter(action.output, stdout, stderr)); err != nil {
				return err
			}
		case triggerWriteHeapProfile:
			if err := writeHeapProfileToDirectory(action.output.path, action.forceGC); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported diagnostics trigger action %d", action.typeID)
		}
	}
	return nil
}

func outputWriter(output compiledOutput, stdout, stderr io.Writer) io.Writer {
	if output.kind == outputStdout {
		return stdout
	}
	return stderr
}

type Call struct {
	matched [maxMatchedRules]matchedRule
	count   int
}

type matchedRule struct {
	rule        *compiledRule
	observation metricsObservation
}

// Begin applies matching pre-operation diagnostic actions. It is allocation-free
// after configuration is installed, provided the caller supplies an existing key.
func Begin(point, key string) (Call, error) {
	current := activeEngine.Load()
	if current == nil {
		return Call{}, nil
	}
	rules := current.rules[point]
	if len(rules) == 0 {
		return Call{}, nil
	}
	var call Call
	for _, rule := range rules {
		if !rule.matches(key) {
			continue
		}
		if call.count == len(call.matched) {
			return Call{}, fmt.Errorf("diagnostics operation %q exceeded %d matching rules", point, maxMatchedRules)
		}
		matched := &call.matched[call.count]
		matched.rule = rule
		switch rule.action {
		case actionDelay:
			timeSleep(rule.delay)
		case actionError:
			if !rule.postError {
				return Call{}, rule.err
			}
		case actionMetrics:
			matched.observation = beginMetricsObservation(rule.metric)
		}
		call.count++
	}
	return call, nil
}

// End applies matching post-operation actions and returns the final result.
func (call *Call) End(err error) error {
	for index := call.count - 1; index >= 0; index-- {
		matched := &call.matched[index]
		switch matched.rule.action {
		case actionMetrics:
			endMetricsObservation(matched.rule.metric, matched.observation, err)
		case actionError:
			if matched.rule.postError && err == nil {
				err = matched.rule.err
			}
		}
	}
	return err
}

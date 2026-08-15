package diagnostics

const EnvVar = "GATEHOUSE_DIAG"

type Config struct {
	Version  int                     `json:"version" yaml:"version"`
	Outputs  map[string]OutputConfig `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	Rules    []RuleConfig            `json:"rules" yaml:"rules"`
	Triggers []TriggerConfig         `json:"triggers,omitempty" yaml:"triggers,omitempty"`
}

type OutputConfig struct {
	Type string `json:"type" yaml:"type"`
	Path string `json:"path,omitempty" yaml:"path,omitempty"`
}

type RuleConfig struct {
	ID      string       `json:"id" yaml:"id"`
	Enabled *bool        `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Op      string       `json:"op" yaml:"op"`
	Key     string       `json:"key" yaml:"key"`
	When    WhenConfig   `json:"when" yaml:"when"`
	Action  ActionConfig `json:"action" yaml:"action"`
}

type WhenConfig struct {
	Mode  string `json:"mode" yaml:"mode"`
	X     int64  `json:"x,omitempty" yaml:"x,omitempty"`
	Limit int64  `json:"limit,omitempty" yaml:"limit,omitempty"`
}

type ActionConfig struct {
	Type    string               `json:"type" yaml:"type"`
	Phase   string               `json:"phase,omitempty" yaml:"phase,omitempty"`
	Error   string               `json:"error,omitempty" yaml:"error,omitempty"`
	DelayMS int64                `json:"delay_ms,omitempty" yaml:"delay_ms,omitempty"`
	Capture MetricsCaptureConfig `json:"capture,omitempty" yaml:"capture,omitempty"`
}

type MetricsCaptureConfig struct {
	Calls   *bool `json:"calls,omitempty" yaml:"calls,omitempty"`
	Errors  *bool `json:"errors,omitempty" yaml:"errors,omitempty"`
	Timing  *bool `json:"timing,omitempty" yaml:"timing,omitempty"`
	Allocs  *bool `json:"allocs,omitempty" yaml:"allocs,omitempty"`
}

type TriggerConfig struct {
	ID      string                `json:"id" yaml:"id"`
	Source  TriggerSourceConfig   `json:"source" yaml:"source"`
	Actions []TriggerActionConfig `json:"actions" yaml:"actions"`
}

type TriggerSourceConfig struct {
	Type   string `json:"type" yaml:"type"`
	Signal string `json:"signal,omitempty" yaml:"signal,omitempty"`
}

type TriggerActionConfig struct {
	Type   string   `json:"type" yaml:"type"`
	Rules  []string `json:"rules,omitempty" yaml:"rules,omitempty"`
	Output string   `json:"output" yaml:"output"`
	GC     string   `json:"gc,omitempty" yaml:"gc,omitempty"`
}

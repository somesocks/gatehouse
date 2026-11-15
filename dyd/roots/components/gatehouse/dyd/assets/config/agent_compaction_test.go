package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gatehouse/configschema"
)

func TestAgentModelCompactionUnion(t *testing.T) {
	for _, test := range []struct {
		name, fields string
		valid        bool
	}{
		{"mcmtr", "algorithm: mcmtr", true},
		{"tadw", "algorithm: tadw\n      context_max_bytes: 8192\n      history_max_bytes: 4096\n      history_min_bytes: 1024", true},
		{"tadw exact integers", "algorithm: tadw\n      context_max_bytes: 9223372036854775807\n      history_max_bytes: 9007199254740993\n      history_min_bytes: 1", true},
		{"unknown", "algorithm: other", false},
		{"missing TADW limit", "algorithm: tadw\n      context_max_bytes: 8192\n      history_max_bytes: 4096", false},
		{"zero minimum", "algorithm: tadw\n      context_max_bytes: 8192\n      history_max_bytes: 4096\n      history_min_bytes: 0", false},
		{"minimum above maximum", "algorithm: tadw\n      context_max_bytes: 8192\n      history_max_bytes: 4096\n      history_min_bytes: 5000", false},
		{"history above context", "algorithm: tadw\n      context_max_bytes: 8192\n      history_max_bytes: 9000\n      history_min_bytes: 1024", false},
		{"mixed mcmtr", "algorithm: mcmtr\n      context_max_bytes: 8192", false},
		{"mixed tadw", "algorithm: tadw\n      history_bytes: 4096\n      context_max_bytes: 8192\n      history_max_bytes: 4096\n      history_min_bytes: 1024", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			contents := `api_version: v1
agent_providers:
  - alias: test
    revision: 1
    protocol: openai-responses
    endpoint: https://example.invalid/responses
    api_key:
      sources: [env:TEST_API_KEY]
agent_models:
  - alias: assistant
    revision: 1
    provider: test
    model: test-model
    parameters: {}
    compaction:
      ` + test.fields + "\n"
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			err, document := ValidateFile(path)
			if err != nil {
				if test.valid {
					t.Fatal(err)
				}
				return
			}
			err, state := ResolveState(document)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
			if !test.valid {
				return
			}
			selected := document.AgentModels
			if selected == nil || len(*selected) != 1 {
				t.Fatal("missing generated model")
			}
			union := (*selected)[0].Compaction
			if test.name == "tadw exact integers" && (state.AgentModels[0].Compaction.TADW.ContextMaxBytes != math.MaxInt64 || state.AgentModels[0].Compaction.TADW.HistoryMaxBytes != 9007199254740993) {
				t.Fatal("integer precision lost", state.AgentModels[0].Compaction)
			}
			if strings.HasPrefix(test.name, "tadw") && (union.Kind != configschema.AgentModelCompactionKindTADWConfig || union.TADWConfig == nil) {
				t.Fatal("not the named TADW branch", union)
			}
			encoded, err := json.Marshal(state.AgentModels[0].Compaction)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := ParseAgentModelCompaction(string(encoded))
			if err != nil || !reflect.DeepEqual(decoded, state.AgentModels[0].Compaction) {
				t.Fatal(string(encoded), decoded, err)
			}
			if strings.Contains(string(encoded), `"kind"`) || strings.Contains(string(encoded), `"MCMTR"`) || strings.Contains(string(encoded), `"TADW"`) {
				t.Fatal("wire configuration must use flat algorithm discriminator", string(encoded))
			}
		})
	}
}

func TestEffectiveAgentModelCompactionValidation(t *testing.T) {
	for _, encoded := range []string{
		`{}`, `{"algorithm":"unknown"}`, `{"algorithm":"mcmtr","history_bytes":100,"buffer_bytes":51}`,
		`{"algorithm":"mcmtr","history_bytes":100,"buffer_bytes":50,"context_max_bytes":0}`,
		`{"algorithm":"tadw","context_max_bytes":100,"history_max_bytes":90,"history_min_bytes":1,"buffer_bytes":0}`,
		`{"algorithm":"tadw","context_max_bytes":100,"history_max_bytes":90}`, `{"algorithm":"tadw","context_max_bytes":100.5,"history_max_bytes":90,"history_min_bytes":1}`,
	} {
		if _, err := ParseAgentModelCompaction(encoded); err == nil {
			t.Fatal("accepted invalid effective configuration", encoded)
		}
	}
	encoded := fmt.Sprintf(`{"algorithm":"tadw","context_max_bytes":%d,"history_max_bytes":9007199254740993,"history_min_bytes":1}`, int64(math.MaxInt64))
	value, err := ParseAgentModelCompaction(encoded)
	if err != nil || value.TADW.ContextMaxBytes != math.MaxInt64 || value.TADW.HistoryMaxBytes != 9007199254740993 {
		t.Fatal(value, err)
	}
	if _, err := json.Marshal(AgentModelCompaction{}); err == nil {
		t.Fatal("accepted empty union")
	}
	if _, err := json.Marshal(AgentModelCompaction{MCMTR: &MCMTRConfig{HistoryBytes: 100, BufferBytes: 50}, TADW: value.TADW}); err == nil {
		t.Fatal("accepted two branches")
	}
}

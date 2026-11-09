package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gatehouse/model"
)

// Synthetic fragments isolate byte arithmetic; renderer-backed tests below
// exercise the same policy with real XML, escaping, metadata and separators.
func tadwHistoryWithSizes(sizes ...int) []tadwRenderedEvent {
	var result []tadwRenderedEvent
	for index, size := range sizes {
		result = append(result, tadwRenderedEvent{ID: fmt.Sprintf("e%d", index), XML: strings.Repeat("x", size)})
	}
	return result
}

func TestSelectTADWHistoryWindow(t *testing.T) {
	standard := tadwWindowConfig{ContextMaxBytes: 40, HistoryMaxBytes: 25, HistoryMinBytes: 12}
	for _, test := range []struct {
		name    string
		sizes   []int
		cached  []string
		config  tadwWindowConfig
		start   int
		bytes   int64
		matched bool
	}{
		{"miss rounds up to a whole event", []int{3, 4, 5, 6, 7}, nil, standard, 3, 13, false},
		{"oldest base keeps full window", []int{3, 4, 5, 6, 7}, []string{"e0"}, standard, 0, 25, true},
		{"oldest eligible base wins regardless of hint order", []int{3, 4, 5, 6, 7}, []string{"e3", "e1", "e4"}, standard, 1, 22, true},
		{"too recent base is rejected", []int{3, 4, 5, 6, 7}, []string{"e4"}, standard, 3, 13, false},
		{"stale empty and duplicate hints", []int{3, 4, 5, 6, 7}, []string{"gone", "", "e3", "e3"}, standard, 3, 13, true},
		{"exact minimum is eligible", []int{2, 3, 5, 7}, []string{"e2"}, standard, 2, 12, true},
		{"one byte below minimum is ineligible", []int{2, 3, 5, 7}, []string{"e2"}, tadwWindowConfig{40, 25, 13}, 1, 15, false},
		{"short history is kept", []int{3, 4}, nil, standard, 0, 7, false},
		{"short history can reuse its first base", []int{3, 4}, []string{"e0"}, standard, 0, 7, true},
		{"short history cannot be shrunk to a later base", []int{3, 4}, []string{"e1"}, standard, 0, 7, false},
		{"empty history", nil, []string{"", "gone"}, standard, 0, 0, false},
		{"large crossing event is not split or skipped", []int{80, 20}, []string{"e1"}, tadwWindowConfig{140, 100, 50}, 0, 100, false},
		{"large recent event overshoots target", []int{1, 1, 98}, nil, tadwWindowConfig{140, 100, 50}, 2, 98, false},
		{"equal minimum and maximum are allowed", []int{3, 4, 5, 6, 7}, nil, tadwWindowConfig{40, 25, 25}, 0, 25, false},
		{"history may consume entire context", []int{3, 4, 5, 6, 7}, []string{"e0"}, tadwWindowConfig{25, 25, 12}, 0, 25, true},
		{"one byte target still retains a whole event", []int{3, 4, 5, 6, 7}, nil, tadwWindowConfig{40, 25, 1}, 4, 7, false},
		{"int64 maximum budget", []int{3, 4, 5, 6, 7}, nil, tadwWindowConfig{math.MaxInt64, math.MaxInt64, 12}, 3, 13, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := tadwHistoryWithSizes(test.sizes...)
			got, err := selectTADWHistoryWindow(candidate, test.cached, test.config)
			if err != nil {
				t.Fatal(err)
			}
			want := tadwHistoryWindow{
				HistoryBytes: test.bytes, ToolingBudgetBytes: test.config.ContextMaxBytes - test.bytes,
				CacheBaseMatched: test.matched,
			}
			for _, event := range candidate[test.start:] {
				want.EventIDs = append(want.EventIDs, event.ID)
			}
			if len(want.EventIDs) > 0 {
				want.BaseEventID = want.EventIDs[0]
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("selectTADWHistoryWindow() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestSelectTADWHistoryWindowUsesRenderedBytes(t *testing.T) {
	message := tadwTestEvent("message.text", map[string]interface{}{"text": "é🙂汉 <question> & answer"})
	message.Ref.Id = "z-message"
	request := tadwTestEvent("agent.request", map[string]interface{}{"agent": "agent"})
	request.Ref.Id = "m-request"
	reply := tadwTestEvent("agent.success", map[string]interface{}{
		"text": "See <report>.",
		"attachments": []interface{}{map[string]interface{}{
			"id": "file", "name": "汉 &.txt", "media_type": "text/plain", "size": int64(math.MaxInt64),
		}},
	})
	reply.Ref.Id = "a-reply"
	transcript, err := renderTADWTranscript([]model.SessionEvent{message, request, reply}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The exact minimum includes two real fragments, their XML overhead, UTF-8
	// and newlines. The referenced file's enormous size is metadata, not weight.
	minimum := int64(len(transcript.History[1].XML + transcript.History[2].XML))
	config := tadwWindowConfig{
		ContextMaxBytes: int64(len(transcript.String())) + 97,
		HistoryMaxBytes: int64(len(transcript.String())), HistoryMinBytes: minimum,
	}
	got, err := selectTADWHistoryWindow(transcript.History, []string{reply.Ref.Id, request.Ref.Id}, config)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.EventIDs, []string{request.Ref.Id, reply.Ref.Id}) || got.BaseEventID != request.Ref.Id || !got.CacheBaseMatched || got.HistoryBytes != minimum || got.ToolingBudgetBytes != config.ContextMaxBytes-minimum {
		t.Fatalf("rendered-byte selection = %+v", got)
	}
	config.HistoryMinBytes++
	got, err = selectTADWHistoryWindow(transcript.History, []string{request.Ref.Id}, config)
	if err != nil || got.CacheBaseMatched || got.BaseEventID != message.Ref.Id || got.HistoryBytes != transcript.Bytes() {
		t.Fatalf("one-byte boundary = (%+v, %v)", got, err)
	}
}

func TestSelectTADWHistoryWindowKeepsBaseUntilRollover(t *testing.T) {
	var all []tadwRenderedEvent
	for index := 0; index < 14; index++ {
		event := tadwTestEvent("message.text", map[string]interface{}{"text": "A new message."})
		event.Ref.Id = fmt.Sprintf("m%02d", index)
		rendered, include, err := renderTADWEvent(event)
		if err != nil || !include {
			t.Fatalf("render fixture: include=%v err=%v", include, err)
		}
		all = append(all, rendered)
	}
	unit := all[0].Bytes()
	config := tadwWindowConfig{ContextMaxBytes: 12 * unit, HistoryMaxBytes: 8 * unit, HistoryMinBytes: 4 * unit}
	first, err := selectTADWHistoryWindow(all[:8], nil, config)
	if err != nil || first.BaseEventID != "m04" || first.HistoryBytes != 4*unit || first.CacheBaseMatched {
		t.Fatalf("initial rollover target = (%+v, %v)", first, err)
	}
	cached := []string{first.BaseEventID}
	previous := first
	for step := 1; step <= 6; step++ {
		// Only the newest eight equal-sized events enter the pure policy.
		got, err := selectTADWHistoryWindow(all[step:step+8], cached, config)
		if err != nil {
			t.Fatal(err)
		}
		wantBase, wantUnits, wantMatch := "m04", int64(4+step), true
		if step >= 5 {
			wantBase, wantUnits, wantMatch = "m09", int64(step-1), step != 5
		}
		if got.BaseEventID != wantBase || got.HistoryBytes != wantUnits*unit || got.CacheBaseMatched != wantMatch || got.ToolingBudgetBytes != config.ContextMaxBytes-got.HistoryBytes {
			t.Fatalf("step %d = %+v, want base=%s units=%d match=%v", step, got, wantBase, wantUnits, wantMatch)
		}
		if got.CacheBaseMatched && !strings.HasPrefix(tadwSelectedXML(all, got.EventIDs), tadwSelectedXML(all, previous.EventIDs)) {
			t.Fatalf("step %d changed the reused prefix", step)
		}
		cached = append(cached, got.BaseEventID)
		previous = got
	}
}

func tadwSelectedXML(events []tadwRenderedEvent, ids []string) string {
	byID := make(map[string]string, len(events))
	for _, event := range events {
		byID[event.ID] = event.XML
	}
	var result strings.Builder
	for _, id := range ids {
		result.WriteString(byID[id])
	}
	return result.String()
}

func TestSelectTADWHistoryWindowDoesNotMutateOrAliasInputs(t *testing.T) {
	candidate := tadwHistoryWithSizes(3, 4, 5, 6, 7)
	cached := []string{"e3", "e1", "e3", "missing"}
	beforeCandidate, beforeCached := slices.Clone(candidate), slices.Clone(cached)
	config := tadwWindowConfig{40, 25, 12}
	first, err := selectTADWHistoryWindow(candidate, cached, config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := selectTADWHistoryWindow(candidate, cached, config)
	if err != nil || !reflect.DeepEqual(first, second) || !reflect.DeepEqual(candidate, beforeCandidate) || !reflect.DeepEqual(cached, beforeCached) {
		t.Fatalf("nondeterministic or mutating policy: %+v, %+v, %v", first, second, err)
	}
	first.EventIDs[0] = "changed"
	if !reflect.DeepEqual(candidate, beforeCandidate) || !reflect.DeepEqual(cached, beforeCached) || second.EventIDs[0] != "e1" {
		t.Fatal("result IDs alias an input or another result")
	}
}

func TestSelectTADWHistoryWindowRejectsInvalidInputs(t *testing.T) {
	for _, config := range []tadwWindowConfig{
		{}, {40, 25, 0}, {40, 25, -1}, {40, 0, 1}, {0, 25, 12},
		{40, 25, 26}, {24, 25, 12}, {-1, 25, 12}, {math.MinInt64, math.MaxInt64, 1},
	} {
		got, err := selectTADWHistoryWindow(nil, nil, config)
		if err == nil || !reflect.DeepEqual(got, tadwHistoryWindow{}) {
			t.Fatalf("invalid config %+v accepted: %+v, %v", config, got, err)
		}
	}
	for _, test := range []struct {
		name      string
		candidate []tadwRenderedEvent
	}{
		{"empty ID", []tadwRenderedEvent{{XML: "x"}}},
		{"empty fragment", []tadwRenderedEvent{{ID: "a"}}},
		{"duplicate IDs", []tadwRenderedEvent{{ID: "a", XML: "x"}, {ID: "a", XML: "y"}}},
		{"oversized list", tadwHistoryWithSizes(6, 5)},
		{"oversized event", tadwHistoryWithSizes(11)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := selectTADWHistoryWindow(test.candidate, []string{"a", "e0"}, tadwWindowConfig{20, 10, 5})
			if err == nil || !reflect.DeepEqual(got, tadwHistoryWindow{}) {
				t.Fatalf("invalid candidates accepted: %+v, %v", got, err)
			}
		})
	}
	var config tadwWindowConfig
	if err := json.Unmarshal([]byte(`{"context_max_bytes":9223372036854775807,"history_max_bytes":25,"history_min_bytes":12}`), &config); err != nil || config.ContextMaxBytes != math.MaxInt64 || config.HistoryMaxBytes != 25 || config.HistoryMinBytes != 12 || config.validate() != nil {
		t.Fatalf("configuration names/int64 range: %+v, %v", config, err)
	}
}

func TestSelectTADWHistoryWindowAgainstSuffixOracle(t *testing.T) {
	random := rand.New(rand.NewPCG(54, 25))
	for trial := 0; trial < 1000; trial++ {
		var candidate []tadwRenderedEvent
		var cached []string
		var total int64
		for count := random.IntN(33); count > 0; count-- {
			size := random.IntN(200) + 1
			event := tadwRenderedEvent{ID: fmt.Sprintf("e%02d", count), XML: strings.Repeat("x", size)}
			candidate = append(candidate, event)
			total += int64(size)
			if random.IntN(2) == 0 {
				cached = append(cached, event.ID, event.ID)
			}
		}
		cached = append(cached, "stale", "")
		random.Shuffle(len(cached), func(a, b int) { cached[a], cached[b] = cached[b], cached[a] })
		maximum := total + random.Int64N(100) + 1
		config := tadwWindowConfig{ContextMaxBytes: maximum + random.Int64N(500), HistoryMaxBytes: maximum, HistoryMinBytes: random.Int64N(maximum) + 1}
		got, err := selectTADWHistoryWindow(candidate, cached, config)
		if err != nil {
			t.Fatal(err)
		}
		want := tadwHistorySuffixOracle(candidate, cached, config)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d: got %+v, want %+v; config=%+v cached=%v", trial, got, want, config, cached)
		}
	}
}

// Deliberately enumerate and concatenate all suffixes instead of using the
// production running-sum scans. This is an independent, small-fixture oracle.
func tadwHistorySuffixOracle(candidate []tadwRenderedEvent, cached []string, config tadwWindowConfig) tadwHistoryWindow {
	if len(candidate) == 0 {
		return tadwHistoryWindow{ToolingBudgetBytes: config.ContextMaxBytes}
	}
	var whole strings.Builder
	for _, event := range candidate {
		whole.WriteString(event.XML)
	}
	minimum := min(config.HistoryMinBytes, int64(whole.Len()))
	var suffixBytes []int64
	for start := range candidate {
		var suffix strings.Builder
		for _, event := range candidate[start:] {
			suffix.WriteString(event.XML)
		}
		suffixBytes = append(suffixBytes, int64(suffix.Len()))
	}
	chosen, matched := 0, false
	for start, size := range suffixBytes {
		if size >= minimum && slices.Contains(cached, candidate[start].ID) {
			chosen, matched = start, true
			break
		}
	}
	if !matched {
		for start, size := range suffixBytes {
			if size >= minimum {
				chosen = start
			}
		}
	}
	result := tadwHistoryWindow{
		BaseEventID: candidate[chosen].ID, HistoryBytes: suffixBytes[chosen],
		ToolingBudgetBytes: config.ContextMaxBytes - suffixBytes[chosen], CacheBaseMatched: matched,
	}
	for _, event := range candidate[chosen:] {
		result.EventIDs = append(result.EventIDs, event.ID)
	}
	return result
}

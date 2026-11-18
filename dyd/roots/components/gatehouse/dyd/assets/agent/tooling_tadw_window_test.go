package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func tadwTideFragment(id string, size int) tadwRenderedEvent {
	return tadwRenderedEvent{ID: id, XML: strings.Repeat("x", size)}
}

func tadwTideNext(events []tadwRenderedEvent, calls *int) func() (tadwRenderedEvent, bool, error) {
	at := 0
	return func() (tadwRenderedEvent, bool, error) {
		*calls = *calls + 1
		if at == len(events) {
			return tadwRenderedEvent{}, false, nil
		}
		event := events[at]
		at++
		return event, true, nil
	}
}

func TestTADWToolingRolloverPurePolicy(t *testing.T) {
	for _, test := range []struct {
		name         string
		root, budget int
		sizes        []int
		count, calls int
		failure      bool
	}{
		{"empty", 10, 100, nil, 0, 1, false},
		{"exact-half", 10, 100, []int{20, 20, 20}, 2, 2, false},
		{"round-up", 10, 100, []int{15, 15, 15, 15}, 3, 3, false},
		{"round-down-at-hard-limit", 10, 100, []int{20, 80, 1}, 1, 2, false},
		{"large-newest-fits", 10, 100, []int{90, 1}, 1, 1, false},
		{"large-newest-does-not-fit", 10, 100, []int{91, 1}, 0, 1, true},
		{"root-over-half", 60, 100, []int{5, 5}, 1, 1, false},
		{"root-fills-budget", 100, 100, []int{1}, 0, 1, true},
		{"root-too-large", 101, 100, []int{1}, 0, 0, true},
		{"odd-budget", 10, 101, []int{20, 20, 20}, 2, 2, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := tadwTideFragment("root", test.root)
			var reverse []tadwRenderedEvent
			for index, size := range test.sizes {
				reverse = append(reverse, tadwTideFragment(fmt.Sprint(index), size))
			}
			calls := 0
			got, err := selectTADWToolingRollover(root, tadwTideNext(reverse, &calls), int64(test.budget))
			if calls != test.calls {
				t.Fatal("unexpected read beyond target/crossing", calls, test.calls)
			}
			if test.failure {
				var budget *tadwToolingBudgetError
				if !errors.As(err, &budget) || got != nil {
					t.Fatal(got, err)
				}
				return
			}
			want := slices.Clone(reverse[:test.count])
			slices.Reverse(want)
			want = append([]tadwRenderedEvent{root}, want...)
			if err != nil || !slices.Equal(got, want) || tadwRenderedBytes(got) > int64(test.budget) {
				t.Fatal(got, want, err)
			}
		})
	}
}

// Independent oracle enumerates every newest suffix and chooses the shortest
// one reaching half, or the largest whole suffix that fits below half.
func tadwTideOracle(root tadwRenderedEvent, chronological []tadwRenderedEvent, budget int64) ([]tadwRenderedEvent, bool) {
	if root.Bytes() > budget {
		return nil, false
	}
	if len(chronological) == 0 {
		return []tadwRenderedEvent{root}, true
	}
	best := -1
	for count := 1; count <= len(chronological); count++ {
		var used int64 = root.Bytes()
		for _, fragment := range chronological[len(chronological)-count:] {
			used += fragment.Bytes()
		}
		if used > budget {
			break
		}
		best = count
		if used >= budget/2 {
			break
		}
	}
	if best < 0 {
		return nil, false
	}
	return append([]tadwRenderedEvent{root}, chronological[len(chronological)-best:]...), true
}

func TestTADWToolingRolloverRandomOracleAndErrors(t *testing.T) {
	random := rand.New(rand.NewPCG(51, 97))
	for iteration := 0; iteration < 1000; iteration++ {
		root := tadwTideFragment("root", 1+random.IntN(60))
		budget := int64(1 + random.IntN(500))
		var forward []tadwRenderedEvent
		count := random.IntN(30)
		for index := 0; index < count; index++ {
			forward = append(forward, tadwTideFragment(fmt.Sprint(index), 1+random.IntN(120)))
		}
		want, ok := tadwTideOracle(root, forward, budget)
		reverse := slices.Clone(forward)
		slices.Reverse(reverse)
		calls := 0
		got, err := selectTADWToolingRollover(root, tadwTideNext(reverse, &calls), budget)
		if ok != (err == nil) || ok && !slices.Equal(got, want) || !ok && got != nil {
			t.Fatalf("iteration %d: got %v (%v), want %v", iteration, got, err, want)
		}
	}
	root := tadwTideFragment("root", 1)
	for _, invalid := range []tadwRenderedEvent{{ID: "bad"}, {XML: "x"}, root} {
		calls := 0
		if got, err := selectTADWToolingRollover(root, tadwTideNext([]tadwRenderedEvent{invalid}, &calls), 100); err == nil || got != nil {
			t.Fatal(got, err)
		}
	}
	failure := errors.New("source failed")
	if got, err := selectTADWToolingRollover(root, func() (tadwRenderedEvent, bool, error) { return tadwRenderedEvent{}, false, failure }, 100); !errors.Is(err, failure) || got != nil {
		t.Fatal(got, err)
	}
	calls := 0
	if got, err := selectTADWToolingRollover(root, tadwTideNext([]tadwRenderedEvent{tadwTideFragment("tail", 2)}, &calls), math.MaxInt64); err != nil || tadwRenderedBytes(got) != 3 {
		t.Fatal(got, err)
	}
}

func tadwTideBudget(initial tadwInitialHistory, budget int64) (tadwInitialHistory, tadwWindowConfig) {
	initial.Window.ToolingBudgetBytes = budget
	return initial, tadwWindowConfig{ContextMaxBytes: initial.Window.HistoryBytes + budget, HistoryMaxBytes: initial.Window.HistoryBytes, HistoryMinBytes: 1}
}

func TestLoadTADWToolingWindowGrowthRolloverAndRecovery(t *testing.T) {
	store, initial, cfg, request, agent := tadwToolingDB(t)
	all, err := loadTADWContext(context.Background(), store, initial, request, agent, tadwToolingThinking(request, 40), cfg)
	if err != nil {
		t.Fatal(err)
	}
	initial, cfg = tadwTideBudget(initial, all.ToolingBytes()-1)
	frozen := slices.Clone(initial.History)
	from := ""
	var priorFrom string
	for _, attempt := range []int{11, 40, 60} {
		thinking := tadwToolingThinking(request, attempt)
		strict, strictErr := loadTADWContextFrom(context.Background(), store, initial, request, agent, thinking, cfg, from, 0)
		got, err := loadTADWToolingWindow(context.Background(), store, initial, request, agent, thinking, cfg, from, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Transcript.History, frozen) || got.Transcript.Bytes() > cfg.ContextMaxBytes || got.Transcript.Bytes() != int64(len(got.Transcript.String())) {
			t.Fatal(got)
		}
		if strictErr == nil {
			if got.FromEventID != from || !reflect.DeepEqual(got.Transcript, strict) {
				t.Fatal("rolled a fitting window", got)
			}
		} else {
			unbounded := cfg
			unbounded.ContextMaxBytes = cfg.ContextMaxBytes + 10000
			history := initial
			history.Window.ToolingBudgetBytes = unbounded.ContextMaxBytes - history.Window.HistoryBytes
			candidate, err := loadTADWContextFrom(context.Background(), store, history, request, agent, thinking, unbounded, from, 0)
			if err != nil {
				t.Fatal(err)
			}
			want, ok := tadwTideOracle(candidate.Tooling[0], candidate.Tooling[1:], initial.Window.ToolingBudgetBytes)
			if !ok || !slices.Equal(got.Transcript.Tooling, want) || got.FromEventID != want[1].ID || got.FromEventID == from {
				t.Fatal("incorrect rollover", got, want)
			}
		}
		recovered, err := loadTADWContextFrom(context.Background(), store, initial, request, agent, thinking, cfg, got.FromEventID, 0)
		if err != nil || !reflect.DeepEqual(recovered, got.Transcript) {
			t.Fatal("recorded window did not rehydrate", recovered, err)
		}
		priorFrom, from = from, got.FromEventID
	}
	if from == "" {
		t.Fatal("fixture never rolled")
	}
	// Shrinking data must not bring dropped nodes back or move the checkpoint.
	if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, tadwToolingThinking(request, 20).Id); err != nil {
		t.Fatal(err)
	}
	got, err := loadTADWToolingWindow(context.Background(), store, initial, request, agent, tadwToolingThinking(request, 60), cfg, from, 0)
	if err != nil || got.FromEventID != from {
		t.Fatal("dropped prefix decoded/reintroduced", got, err, priorFrom)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := store.PingContext(ctx); err != nil {
		t.Fatal("retained connection", err)
	}
}

func TestLoadTADWToolingWindowLimitsAndFailures(t *testing.T) {
	for _, name := range []string{"exact", "root-too-large", "newest-too-large", "large-older", "missing-base", "missing-file", "corrupt-retained", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			store, initial, cfg, request, agent := tadwToolingDB(t)
			thinking := tadwToolingThinking(request, 40)
			all, err := loadTADWContext(context.Background(), store, initial, request, agent, thinking, cfg)
			if err != nil {
				t.Fatal(err)
			}
			budget := all.ToolingBytes() - 1
			from := ""
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "exact":
				budget = all.ToolingBytes()
			case "root-too-large":
				budget = all.Tooling[0].Bytes() - 1
			case "missing-base":
				from = tadwToolingThinking(request, 999).Id
			case "newest-too-large", "large-older":
				id := 31
				field := "output"
				if name == "large-older" {
					id = 20
					field = "code"
				}
				// Preserve the other required tool.request fields.
				if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload=json_set(payload,?,?) WHERE id=?`, "$."+field, strings.Repeat("x", int(budget)+1), tadwToolingThinking(request, id).Id); err != nil {
					t.Fatal(err)
				}
			case "missing-file":
				if _, err := store.Exec(`DELETE FROM gatehouse_session_files`); err != nil {
					t.Fatal(err)
				}
			case "corrupt-retained":
				if _, err := store.Exec(`UPDATE gatehouse_session_events SET payload='invalid JSON' WHERE id=?`, tadwToolingThinking(request, 31).Id); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				cancel()
			}
			initial, cfg = tadwTideBudget(initial, budget)
			got, err := loadTADWToolingWindow(ctx, store, initial, request, agent, thinking, cfg, from, 0)
			if name == "exact" || name == "large-older" {
				if err != nil || got.Transcript.Bytes() > cfg.ContextMaxBytes {
					t.Fatal(got, err)
				}
				if name == "exact" && got.FromEventID != "" {
					t.Fatal("exact fit rolled")
				}
			} else if err == nil || !reflect.DeepEqual(got, tadwToolingWindow{}) {
				t.Fatal("expected no partial window", got, err)
			}
			if name == "newest-too-large" {
				var overflow *tadwToolingBudgetError
				if !errors.As(err, &overflow) || overflow.EventID != tadwToolingThinking(request, 31).Id {
					t.Fatal(err)
				}
			}
			check, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := store.PingContext(check); err != nil {
				t.Fatal(err)
			}
		})
	}
}

package agent

import "fmt"

// tadwWindowConfig describes usable transcript space, not the model's token
// limit. It is intentionally separate from the active MCMTR profile for now.
type tadwWindowConfig struct {
	ContextMaxBytes int64 `json:"context_max_bytes"`
	HistoryMaxBytes int64 `json:"history_max_bytes"`
	HistoryMinBytes int64 `json:"history_min_bytes"`
}

func (config tadwWindowConfig) validate() error {
	if config.HistoryMinBytes <= 0 || config.HistoryMaxBytes < config.HistoryMinBytes || config.ContextMaxBytes < config.HistoryMaxBytes {
		return fmt.Errorf("invalid TADW window configuration: require 0 < history_min_bytes <= history_max_bytes <= context_max_bytes (got %d, %d, %d)", config.HistoryMinBytes, config.HistoryMaxBytes, config.ContextMaxBytes)
	}
	return nil
}

// tadwHistoryWindow contains only IDs and byte accounting, never rendered chat.
// CacheBaseMatched means an alignment hint matched, not a confirmed provider hit.
// Persistence of the chosen IDs/base is a separate responsibility.
type tadwHistoryWindow struct {
	EventIDs           []string
	BaseEventID        string
	HistoryBytes       int64
	ToolingBudgetBytes int64
	CacheBaseMatched   bool
}

// selectTADWHistoryWindow selects a contiguous suffix of an already bounded,
// linearized candidate history in prompt order (old edge first, recent edge
// last). It never sorts, splits events, skips ahead, or modifies the input.
//
// The caller must supply no more than HistoryMaxBytes of actual rendered
// fragments. Oversized input is an error, not an invitation to eagerly read a
// whole session and truncate it here. Cached base IDs must be scoped by the
// caller to the same conversation/model/rendering profile.
func selectTADWHistoryWindow(candidate []tadwRenderedEvent, cachedBaseIDs []string, config tadwWindowConfig) (tadwHistoryWindow, error) {
	if err := config.validate(); err != nil {
		return tadwHistoryWindow{}, err
	}

	// Index candidate IDs only. Stale/duplicate cache hints do not grow this map.
	cached := make(map[string]bool, len(candidate))
	var total int64
	for _, event := range candidate {
		if event.ID == "" || event.Bytes() == 0 {
			return tadwHistoryWindow{}, fmt.Errorf("invalid TADW history candidate: require nonempty ID and rendered fragment")
		}
		if _, duplicate := cached[event.ID]; duplicate {
			return tadwHistoryWindow{}, fmt.Errorf("duplicate TADW history candidate ID %q", event.ID)
		}
		// Subtract before comparing so even INT64_MAX limits cannot overflow.
		if event.Bytes() > config.HistoryMaxBytes-total {
			return tadwHistoryWindow{}, fmt.Errorf("TADW candidate history exceeds history_max_bytes; supply an already bounded history")
		}
		total += event.Bytes()
		cached[event.ID] = false
	}
	if len(candidate) == 0 {
		return tadwHistoryWindow{ToolingBudgetBytes: config.ContextMaxBytes}, nil
	}
	for _, id := range cachedBaseIDs {
		if _, exists := cached[id]; exists {
			cached[id] = true
		}
	}

	// Short history is retained in full. Its first event may still be a valid
	// cached base; a later base must not shrink it below the available amount.
	minimum := min(config.HistoryMinBytes, total)
	start := -1
	retained := total
	matched := false
	for index, event := range candidate {
		if retained < minimum {
			break
		}
		if cached[event.ID] {
			start, matched = index, true
			break // The first eligible match retains the most history.
		}
		retained -= event.Bytes()
	}
	if !matched {
		// Rollover to the smallest recent suffix meeting the same minimum.
		// Include the whole crossing event, even if it overshoots the target.
		start, retained = len(candidate), 0
		for retained < minimum {
			start--
			retained += candidate[start].Bytes()
		}
	}

	ids := make([]string, len(candidate)-start)
	for index, event := range candidate[start:] {
		ids[index] = event.ID
	}
	return tadwHistoryWindow{
		EventIDs:           ids,
		BaseEventID:        ids[0],
		HistoryBytes:       retained,
		ToolingBudgetBytes: config.ContextMaxBytes - retained,
		CacheBaseMatched:   matched,
	}, nil
}

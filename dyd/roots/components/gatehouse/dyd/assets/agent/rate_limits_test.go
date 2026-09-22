package agent

import (
	"testing"
	"time"

	"gatehouse/database"
	"gatehouse/model"
)

func TestEstimateAgentRequestTokens(t *testing.T) {
	request := struct {
		Text string `json:"text"`
	}{Text: "test"}
	got, err := estimateAgentRequestTokens(request)
	if err != nil {
		t.Fatal(err)
	}
	if got != 264 {
		t.Fatalf("estimateAgentRequestTokens() = %d, want 264", got)
	}
}

func TestAgentRateLimitWaitUntil(t *testing.T) {
	reservedAt := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	claim := database.AgentRateLimitClaim{ReservedAt: reservedAt, Wait: 30 * time.Second}
	if got, want := agentRateLimitWaitUntil(claim), reservedAt.Add(30*time.Second); !got.Equal(want) {
		t.Fatalf("agentRateLimitWaitUntil() = %s, want %s", got, want)
	}
}

func TestAgentUsageTokensRefundsRetryableProviderFailure(t *testing.T) {
	input, output := 120, 30
	metrics := &model.SessionEventMetrics{InputTokens: &input, OutputTokens: &output}
	refundedInput, refundedOutput := agentUsageTokens(&openAIRetryableError{}, metrics)
	if refundedInput == nil || refundedOutput == nil || *refundedInput != 0 || *refundedOutput != 0 {
		t.Fatalf("agentUsageTokens() retryable failure = (%v, %v), want (0, 0)", refundedInput, refundedOutput)
	}
	settledInput, settledOutput := agentUsageTokens(nil, metrics)
	if settledInput != metrics.InputTokens || settledOutput != metrics.OutputTokens {
		t.Fatalf("agentUsageTokens() successful completion = (%v, %v), want metrics usage", settledInput, settledOutput)
	}
}

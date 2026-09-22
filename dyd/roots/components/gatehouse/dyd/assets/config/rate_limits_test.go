package config

import "testing"

func TestWorkspaceAgentRateLimitsValidation(t *testing.T) {
	limits := &WorkspaceAgentRateLimits{WorkspaceInput: &TokenBucket{MinimumBalance: -120, MaximumBalance: 120, RefillPerMinute: 60}}
	if err := ValidateWorkspaceAgentRateLimits(limits); err != nil {
		t.Fatal(err)
	}
	limits.UserOutput = &TokenBucket{MinimumBalance: 1, MaximumBalance: 120, RefillPerMinute: 1}
	if err := ValidateWorkspaceAgentRateLimits(limits); err == nil {
		t.Fatal("ValidateWorkspaceAgentRateLimits() accepted a positive minimum balance")
	}
}

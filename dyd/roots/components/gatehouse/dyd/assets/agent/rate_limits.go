package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gatehouse/database"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

const (
	agentRateLimitEstimateBytesPerToken = 2
	agentRateLimitEstimateOverhead      = 256
)

func estimateAgentRequestTokens(request any) (int, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("encode rate-limited provider request: %w", err)
	}
	return (len(encoded)+agentRateLimitEstimateBytesPerToken-1)/agentRateLimitEstimateBytesPerToken + agentRateLimitEstimateOverhead, nil
}

func (runtime *SessionEventReplyRuntime) agentRateLimitClaim(ctx dbos.Context, parent model.SessionEventRef, thinking model.SessionEvent, selected *database.WorkspaceAgentModel, principal model.PrincipalRef, request any) (error, database.AgentRateLimitClaim) {
	if selected.RateLimits == nil || selected.RateLimits.Empty() {
		return nil, database.AgentRateLimitClaim{}
	}
	input, err := estimateAgentRequestTokens(request)
	if err != nil {
		return err, database.AgentRateLimitClaim{}
	}
	claim, err := dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, transaction dbos.Tx) (database.AgentRateLimitClaim, error) {
		claimErr, claim := runtime.store.AgentRateLimitClaimInTransaction(step, transaction, *selected, principal, input, selected.MaxOutputTokens)
		return claim, claimErr
	}, dbos.WithStepName("gatehouse.agent-rate-limit-claim"))
	if err != nil {
		return err, claim
	}
	if claim.Wait > 0 {
		until := agentRateLimitWaitUntil(claim)
		if err := runtime.thinkingDelay(ctx, thinking, until); err != nil {
			_ = runtime.agentRateLimitSettle(ctx, claim, intPointer(0), intPointer(0))
			return err, database.AgentRateLimitClaim{}
		}
		if remaining := time.Until(until); remaining > 0 {
			if _, err := dbos.Sleep(ctx, remaining); err != nil {
				_ = runtime.agentRateLimitSettle(ctx, claim, intPointer(0), intPointer(0))
				return err, database.AgentRateLimitClaim{}
			}
		}
	}
	if err := runtime.replyCancellationCheck(ctx, parent); err != nil {
		_ = runtime.agentRateLimitSettle(ctx, claim, intPointer(0), intPointer(0))
		return err, database.AgentRateLimitClaim{}
	}
	return nil, claim
}

func agentRateLimitWaitUntil(claim database.AgentRateLimitClaim) time.Time {
	return claim.ReservedAt.Add(claim.Wait)
}

func (runtime *SessionEventReplyRuntime) agentRateLimitSettle(ctx dbos.Context, claim database.AgentRateLimitClaim, input, output *int) error {
	if claim.Workspace.Id == "" {
		return nil
	}
	_, err := dbos.RunAsTransaction(ctx, runtime.dataSource, func(step context.Context, transaction dbos.Tx) (struct{}, error) {
		return struct{}{}, runtime.store.AgentRateLimitSettleInTransaction(step, transaction, claim, input, output)
	}, dbos.WithStepName("gatehouse.agent-rate-limit-settle"))
	return err
}

func intPointer(value int) *int {
	return &value
}

func agentUsageTokens(providerErr error, metrics *model.SessionEventMetrics) (*int, *int) {
	var retryable *openAIRetryableError
	if errors.As(providerErr, &retryable) {
		return intPointer(0), intPointer(0)
	}
	if metrics == nil {
		return nil, nil
	}
	return metrics.InputTokens, metrics.OutputTokens
}

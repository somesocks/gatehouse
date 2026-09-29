package model

const (
	SessionEventKindMessageText = "message.text"

	SessionEventKindAgentRequest = "agent.request"
	SessionEventKindAgentSuccess = "agent.success"
	SessionEventKindAgentFailure = "agent.failure"

	SessionEventKindThinkingRequest = "thinking.request"
	SessionEventKindThinkingUpdate  = "thinking.update"
	SessionEventKindThinkingSuccess = "thinking.success"
	SessionEventKindThinkingFailure = "thinking.failure"

	SessionEventKindToolRequest = "tool.request"
	SessionEventKindToolSuccess = "tool.success"
	SessionEventKindToolFailure = "tool.failure"

	SessionEventKindApprovalRequest = "approval.request"
	SessionEventKindApprovalSuccess = "approval.success"
	SessionEventKindApprovalFailure = "approval.failure"

	SessionEventKindInputRequest = "input.request"
	SessionEventKindInputSuccess = "input.success"
	SessionEventKindInputFailure = "input.failure"

	SessionEventKindCancelRequest = "cancel.request"
	SessionEventKindCancelSuccess = "cancel.success"
	SessionEventKindCancelFailure = "cancel.failure"
)

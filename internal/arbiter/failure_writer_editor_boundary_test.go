package arbiter

import (
	"strings"
	"testing"
)

func TestFailureDecisionRejectsWriterToEditorRerouteBeforePersistenceBoundary(t *testing.T) {
	facts := FailureFacts{
		Kind:        "worker_failure",
		Agent:       "writer",
		Task:        "写第 2 章",
		Phase:       "writing",
		NextChapter: 2,
	}
	decision := FailureDecision{
		Action:   "reroute",
		Dispatch: &DispatchOp{Agent: "editor", Task: "检查第 2 章并保存评审"},
		Reason:   "writer 没有推进，交给 editor 检查",
	}

	err := decision.ValidateAgainst(facts)
	if err == nil {
		t.Fatal("writer -> editor failure reroute must be rejected before chapter persistence")
	}
	if !strings.Contains(err.Error(), "不能在章节完成前直接改派 editor") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestFailureDecisionRejectsWriterDeadlockToEditorReroute(t *testing.T) {
	facts := FailureFacts{
		Kind:        "deadlock",
		Agent:       "writer",
		Task:        "写第 2 章",
		Repeats:     3,
		Phase:       "writing",
		NextChapter: 2,
	}
	decision := FailureDecision{
		Action:   "reroute",
		Dispatch: &DispatchOp{Agent: "editor", Task: "审阅第 2 章"},
		Reason:   "writer 连续无进展",
	}

	if err := decision.ValidateAgainst(facts); err == nil {
		t.Fatal("writer deadlock -> editor reroute must be rejected")
	}
}

func TestFailureDecisionAllowsBoundedWriterRecoveryReroutes(t *testing.T) {
	facts := FailureFacts{
		Kind:        "worker_failure",
		Agent:       "writer",
		Task:        "写第 2 章",
		Phase:       "writing",
		NextChapter: 2,
	}
	for _, agent := range []string{"writer", "architect_short", "architect_long"} {
		t.Run(agent, func(t *testing.T) {
			decision := FailureDecision{
				Action:   "reroute",
				Dispatch: &DispatchOp{Agent: agent, Task: "恢复 writer 持久化前置条件"},
				Reason:   "使用合法恢复路径",
			}
			if err := decision.ValidateAgainst(facts); err != nil {
				t.Fatalf("legal writer recovery reroute to %s rejected: %v", agent, err)
			}
		})
	}
}

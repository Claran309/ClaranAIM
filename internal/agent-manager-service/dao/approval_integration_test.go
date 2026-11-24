//go:build integration

package dao

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ClaranAIM/internal/agent-manager-service/model"
)

const approvalIntegrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupApprovalRepo(t *testing.T) *agentApprovalRepositoryImpl {
	t.Helper()
	dsn := os.Getenv(approvalIntegrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 manager approval MySQL 集成测试", approvalIntegrationDSNEnv)
	}
	db, err := InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	if err := db.Exec("DELETE FROM agent_approvals").Error; err != nil {
		t.Fatalf("clean agent_approvals error = %v", err)
	}
	return &agentApprovalRepositoryImpl{db: db}
}

func TestIntegrationApprovalPersistsAcrossRepositories(t *testing.T) {
	first := setupApprovalRepo(t)
	ctx := context.Background()
	approval := &model.AgentApproval{
		ApprovalID:     fmt.Sprintf("it-apv-%d", time.Now().UnixNano()),
		RunID:          "run-1",
		InterruptID:    "tool:1",
		SessionID:      "session-1",
		BotID:          71,
		UserID:         1001,
		ConversationID: 2001,
		Status:         "pending",
		ToolName:       "write_file",
		ArgumentsJSON:  `{"path":"a.txt"}`,
		Description:    "write file",
	}
	if err := first.Create(ctx, approval); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	second := &agentApprovalRepositoryImpl{db: first.db}
	got, err := second.Get(ctx, approval.ApprovalID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.Status != "pending" || got.ToolName != "write_file" {
		t.Fatalf("approval from second repo = %#v", got)
	}
	got.Status = "approved"
	got.Reason = "ok"
	got.UpdatedAt = time.Now()
	if err := second.Update(ctx, got); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	list, err := first.List(ctx, AgentApprovalFilter{UserID: 1001, BotID: 71, Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 || list[0].Status != "approved" || list[0].Reason != "ok" {
		t.Fatalf("list = %#v", list)
	}
}

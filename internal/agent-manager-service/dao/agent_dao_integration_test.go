//go:build integration

package dao

import (
	"ClaranAIM/internal/agent-manager-service/model"
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
)

const agentDaoIntegrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupAgentDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(agentDaoIntegrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 agent-manager MySQL 集成测试", agentDaoIntegrationDSNEnv)
	}
	db, err := InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	for _, table := range []string{"bot_permissions", "agent_dispatch_records", "agent_tasks", "billing_records", "agent_audit_records", "agent_subscription_rules", "bots"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clean table %s error = %v", table, err)
		}
	}
	return db
}

func newTestBot(id, ownerID int64) *model.Bot {
	return &model.Bot{
		ID:          id,
		Name:        "it-agent",
		Type:        "assistant",
		OwnerID:     ownerID,
		ModelName:   "glm-4.7",
		IsActive:    true,
		AgentUserID: id + 1000,
	}
}

func TestIntegrationBotRepositoryCRUD(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewBotRepo(db)
	ctx := context.Background()
	bot := newTestBot(7001, 1)
	if err := repo.CreateBot(ctx, bot); err != nil {
		t.Fatalf("CreateBot() error = %v", err)
	}
	got, err := repo.GetBotByID(ctx, bot.ID)
	if err != nil {
		t.Fatalf("GetBotByID() error = %v", err)
	}
	if got == nil || got.Name != "it-agent" || !got.IsActive {
		t.Fatalf("bot = %#v", got)
	}
	got.Name = "renamed"
	if err := repo.UpdateBot(ctx, got); err != nil {
		t.Fatalf("UpdateBot() error = %v", err)
	}
	reloaded, _ := repo.GetBotByID(ctx, bot.ID)
	if reloaded.Name != "renamed" {
		t.Fatalf("name = %q, want renamed", reloaded.Name)
	}
	if err := repo.DeleteBot(ctx, bot.ID); err != nil {
		t.Fatalf("DeleteBot() error = %v", err)
	}
	deleted, err := repo.GetBotByID(ctx, bot.ID)
	if err != nil || deleted != nil {
		t.Fatalf("deleted bot = %#v err=%v", deleted, err)
	}
}

func TestIntegrationBotRepositoryListAndAgentUserLookup(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewBotRepo(db)
	ctx := context.Background()
	if err := repo.CreateBot(ctx, newTestBot(7002, 1)); err != nil {
		t.Fatalf("CreateBot() error = %v", err)
	}
	if err := repo.CreateBot(ctx, newTestBot(7003, 2)); err != nil {
		t.Fatalf("CreateBot() error = %v", err)
	}
	owned, err := repo.ListBots(ctx, 1, "")
	if err != nil {
		t.Fatalf("ListBots() error = %v", err)
	}
	if len(owned) != 1 || owned[0].ID != 7002 {
		t.Fatalf("owned bots = %#v", owned)
	}
	found, err := repo.GetBotByAgentUserID(ctx, 8003)
	if err != nil {
		t.Fatalf("GetBotByAgentUserID() error = %v", err)
	}
	if found == nil || found.ID != 7003 {
		t.Fatalf("agent user lookup = %#v", found)
	}
	missing, err := repo.GetBotByAgentUserID(ctx, 999999)
	if err != nil || missing != nil {
		t.Fatalf("missing lookup = %#v err=%v", missing, err)
	}
}

func TestIntegrationPermissionRepositoryUpsertAndDelete(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewPermissionRepo(db)
	ctx := context.Background()
	permission := &model.BotPermission{BotID: 7001, UserID: 2, Role: "viewer"}
	if err := repo.UpsertPermission(ctx, permission); err != nil {
		t.Fatalf("UpsertPermission() error = %v", err)
	}
	permission.Role = "operator"
	if err := repo.UpsertPermission(ctx, permission); err != nil {
		t.Fatalf("UpsertPermission(update) error = %v", err)
	}
	got, err := repo.GetPermission(ctx, 7001, 2)
	if err != nil {
		t.Fatalf("GetPermission() error = %v", err)
	}
	if got == nil || got.Role != "operator" {
		t.Fatalf("permission = %#v", got)
	}
	list, err := repo.ListPermissions(ctx, 7001)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %#v err=%v", list, err)
	}
	if err := repo.DeletePermission(ctx, 7001, 2); err != nil {
		t.Fatalf("DeletePermission() error = %v", err)
	}
	after, err := repo.GetPermission(ctx, 7001, 2)
	if err != nil || after != nil {
		t.Fatalf("permission after delete = %#v err=%v", after, err)
	}
}

func TestIntegrationDispatchRepositoryIdempotentStart(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewAgentDispatchRepo(db)
	ctx := context.Background()
	record := &model.AgentDispatchRecord{
		EventID:        "it-event-1",
		AgentUserID:    8001,
		BotID:          7001,
		EventType:      "message.created",
		ConversationID: 100,
		SenderID:       1,
		SourceMsgID:    11,
	}
	started, err := repo.Start(ctx, record)
	if err != nil || !started {
		t.Fatalf("first Start() = (%v, %v), want true", started, err)
	}
	if err := repo.MarkCompleted(ctx, "it-event-1", 8001, 22); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}
	again, err := repo.Start(ctx, record)
	if err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if again {
		t.Fatal("completed dispatch must not be started again")
	}
}

func TestIntegrationDispatchRepositoryFailureAllowsRetry(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewAgentDispatchRepo(db)
	ctx := context.Background()
	record := &model.AgentDispatchRecord{
		EventID:        "it-event-2",
		AgentUserID:    8002,
		BotID:          7002,
		EventType:      "message.created",
		ConversationID: 101,
		SenderID:       1,
		SourceMsgID:    12,
	}
	if started, err := repo.Start(ctx, record); err != nil || !started {
		t.Fatalf("first Start() = (%v, %v)", started, err)
	}
	if err := repo.MarkFailed(ctx, "it-event-2", 8002, "transient error"); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	again, err := repo.Start(ctx, record)
	if err != nil {
		t.Fatalf("Start() after failure error = %v", err)
	}
	if !again {
		t.Fatal("failed dispatch should be retryable")
	}
}

func TestIntegrationAgentTaskLifecycle(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewAgentTaskRepo(db)
	ctx := context.Background()
	task := &model.AgentTask{
		BotID:          7001,
		AgentUserID:    8001,
		ConversationID: 100,
		TriggerUserID:  1,
		SourceEventID:  "it-task-1",
		EventType:      "message.created",
		Status:         "queued",
	}
	if err := repo.UpsertQueued(ctx, task); err != nil {
		t.Fatalf("UpsertQueued() error = %v", err)
	}
	if err := repo.MarkRunning(ctx, "it-task-1"); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}
	if err := repo.MarkCompleted(ctx, "it-task-1"); err != nil {
		t.Fatalf("MarkCompleted() error = %v", err)
	}
	var stored model.AgentTask
	if err := db.Where("source_event_id = ?", "it-task-1").First(&stored).Error; err != nil {
		t.Fatalf("load task error = %v", err)
	}
	if stored.Status != "completed" || stored.CompletedAt == nil {
		t.Fatalf("task = %#v", stored)
	}
}

func TestIntegrationBillingRecordPersistence(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewBillingRepo(db)
	ctx := context.Background()
	record := &model.BillingRecord{
		BotID:          7001,
		UserID:         1,
		ConversationID: 100,
		Action:         "chat",
		TokenCount:     30,
		InputTokens:    20,
		OutputTokens:   10,
		Cost:           0.0012,
		ModelName:      "glm-4.7",
	}
	if err := repo.CreateRecord(ctx, record); err != nil {
		t.Fatalf("CreateRecord() error = %v", err)
	}
	records, total, err := repo.ListRecords(ctx, 7001, 1, 10, 0)
	if err != nil {
		t.Fatalf("ListRecords() error = %v", err)
	}
	if total != 1 || len(records) != 1 || records[0].InputTokens != 20 {
		t.Fatalf("records = %#v total=%d", records, total)
	}
}

func TestIntegrationAuditRecordAndSubscriptionRule(t *testing.T) {
	db := setupAgentDB(t)
	auditRepo := NewAgentAuditRepo(db)
	subscriptionRepo := NewAgentSubscriptionRepo(db)
	ctx := context.Background()
	audit := &model.AgentAuditRecord{
		EventID:        "it-audit-1",
		EventType:      "message.created",
		BotID:          7001,
		AgentUserID:    8001,
		ConversationID: 100,
		SenderID:       1,
		Decision:       "trigger",
		Reason:         "mention",
	}
	if err := auditRepo.Create(ctx, audit); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	var stored model.AgentAuditRecord
	if err := db.Where("event_id = ?", "it-audit-1").First(&stored).Error; err != nil {
		t.Fatalf("load audit error = %v", err)
	}
	if stored.Decision != "trigger" || stored.Reason != "mention" {
		t.Fatalf("audit = %#v", stored)
	}

	rule := &model.AgentSubscriptionRule{
		BotID:          7001,
		AgentUserID:    8001,
		ConversationID: 100,
		EventTypes:     "message.created",
		TriggerMode:    "mention",
		IsActive:       true,
		SourceRouteID:  1,
	}
	if err := subscriptionRepo.UpsertRouteMirror(ctx, rule); err != nil {
		t.Fatalf("UpsertRouteMirror() error = %v", err)
	}
	rules, err := subscriptionRepo.ListActiveRules(ctx, 100, "message.created")
	if err != nil {
		t.Fatalf("ListActiveRules() error = %v", err)
	}
	if len(rules) != 1 || rules[0].BotID != 7001 {
		t.Fatalf("rules = %#v", rules)
	}
	if err := subscriptionRepo.DeleteRouteMirror(ctx, 7001, 1); err != nil {
		t.Fatalf("DeleteRouteMirror() error = %v", err)
	}
	after, err := subscriptionRepo.ListActiveRules(ctx, 100, "message.created")
	if err != nil {
		t.Fatalf("ListActiveRules(after) error = %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("rules after delete = %#v", after)
	}
}

func TestIntegrationApprovalStatusMachine(t *testing.T) {
	db := setupAgentDB(t)
	repo := NewAgentApprovalRepo(db)
	ctx := context.Background()
	approval := &model.AgentApproval{
		ApprovalID:     "it-approval-1",
		RunID:          "run-1",
		InterruptID:    "tool:1",
		SessionID:      "session-1",
		BotID:          7001,
		UserID:         1,
		ConversationID: 100,
		Status:         "pending",
		ToolName:       "write_file",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	if err := repo.Create(ctx, approval); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	pending, err := repo.List(ctx, AgentApprovalFilter{UserID: 1, Status: "pending"})
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending list = %#v err=%v", pending, err)
	}
	approval.Status = "approved"
	approval.Reason = "ok"
	if err := repo.Update(ctx, approval); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	approved, err := repo.List(ctx, AgentApprovalFilter{UserID: 1, Status: "approved"})
	if err != nil || len(approved) != 1 || approved[0].Reason != "ok" {
		t.Fatalf("approved list = %#v err=%v", approved, err)
	}
	if left, _ := repo.List(ctx, AgentApprovalFilter{UserID: 1, Status: "pending"}); len(left) != 0 {
		t.Fatalf("pending should be empty, got %#v", left)
	}
}

//go:build integration

package service

import (
	"ClaranAIM/internal/group-service/dao"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
)

const groupIntegrationDSNEnv = "CLARAN_TEST_MYSQL_DSN"

func setupGroupService(t *testing.T) (GroupService, dao.GroupRepository, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv(groupIntegrationDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未设置，跳过 group-service MySQL 集成测试", groupIntegrationDSNEnv)
	}
	db, err := dao.InitDB(dsn)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	// groups 在 MySQL 8 是保留字，清理时必须加反引号，否则会报 1064 语法错误。
	for _, table := range []string{"group_members", "`groups`", "event_outbox"} {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("clean table %s error = %v", table, err)
		}
	}
	repo := dao.NewGroupRepo(db)
	return NewGroupService(repo, nil), repo, db
}

func uniqueGroupID() int64 {
	return time.Now().UnixNano() / 1000
}

func TestIntegrationCreateGroupThenCompensateRemovesGroup(t *testing.T) {
	svc, repo, _ := setupGroupService(t)
	ctx := context.Background()
	groupID := uniqueGroupID()
	group, err := svc.CreateGroupWithID(ctx, groupID, "saga group", 101, []int64{102, 103})
	if err != nil {
		t.Fatalf("CreateGroupWithID() error = %v", err)
	}
	if group.ID != groupID {
		t.Fatalf("group id = %d, want %d", group.ID, groupID)
	}
	members, err := repo.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers() error = %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("members = %d, want 3", len(members))
	}

	if err := svc.DeleteGroup(ctx, groupID, 101); err != nil {
		t.Fatalf("DeleteGroup(compensate) error = %v", err)
	}
	after, err := repo.GetGroupByID(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupByID() error = %v", err)
	}
	if after != nil {
		t.Fatalf("group still exists after compensation: %#v", after)
	}
	members, err = repo.GetGroupMembers(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupMembers(after) error = %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("members after compensation = %d, want 0", len(members))
	}
}

func TestIntegrationCompensationRejectsNonOwner(t *testing.T) {
	svc, repo, _ := setupGroupService(t)
	ctx := context.Background()
	groupID := uniqueGroupID()
	if _, err := svc.CreateGroupWithID(ctx, groupID, "owner group", 201, []int64{202, 203}); err != nil {
		t.Fatalf("CreateGroupWithID() error = %v", err)
	}
	if err := svc.DeleteGroup(ctx, groupID, 999); err == nil {
		t.Fatal("DeleteGroup() by non-owner succeeded, want error")
	}
	group, err := repo.GetGroupByID(ctx, groupID)
	if err != nil {
		t.Fatalf("GetGroupByID() error = %v", err)
	}
	if group == nil {
		t.Fatal("group should survive rejected compensation")
	}
}

func TestIntegrationCompensationIsIdempotent(t *testing.T) {
	svc, _, _ := setupGroupService(t)
	ctx := context.Background()
	groupID := uniqueGroupID()
	if _, err := svc.CreateGroupWithID(ctx, groupID, "idempotent group", 301, []int64{302, 303}); err != nil {
		t.Fatalf("CreateGroupWithID() error = %v", err)
	}
	if err := svc.DeleteGroup(ctx, groupID, 301); err != nil {
		t.Fatalf("first DeleteGroup() error = %v", err)
	}
	err := svc.DeleteGroup(ctx, groupID, 301)
	if err == nil {
		t.Fatal("second DeleteGroup() succeeded, want not-found error")
	}
	if !errors.Is(err, err) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIntegrationCreateGroupRejectsDuplicatePreallocatedID(t *testing.T) {
	svc, _, _ := setupGroupService(t)
	ctx := context.Background()
	groupID := uniqueGroupID()
	if _, err := svc.CreateGroupWithID(ctx, groupID, "first", 401, []int64{402, 403}); err != nil {
		t.Fatalf("first CreateGroupWithID() error = %v", err)
	}
	if _, err := svc.CreateGroupWithID(ctx, groupID, "duplicate", 401, []int64{402, 403}); err == nil {
		t.Fatal("duplicate CreateGroupWithID() succeeded, want error")
	}
}

func TestIntegrationCompensationLeavesOutboxDeletedEvent(t *testing.T) {
	svc, _, db := setupGroupService(t)
	ctx := context.Background()
	groupID := uniqueGroupID()
	if _, err := svc.CreateGroupWithID(ctx, groupID, "outbox group", 501, []int64{502, 503}); err != nil {
		t.Fatalf("CreateGroupWithID() error = %v", err)
	}
	if err := svc.DeleteGroup(ctx, groupID, 501); err != nil {
		t.Fatalf("DeleteGroup() error = %v", err)
	}
	var events []outbox.Event
	if err := db.WithContext(ctx).Where("aggregate_id = ?", groupID).Find(&events).Error; err != nil {
		t.Fatalf("query outbox error = %v", err)
	}
	foundDeleted := false
	for _, event := range events {
		if event.EventType == "group.deleted" {
			foundDeleted = true
		}
	}
	if !foundDeleted {
		t.Fatalf("group.deleted outbox event missing; events = %+v", events)
	}
}

package service

import (
	"ClaranAIM/internal/group-service/model"
	"context"
	"testing"
	"time"
)

type fakeGroupRepo struct {
	groups  map[int64]*model.Group
	members map[int64]map[int64]*model.GroupMember
}

func newFakeGroupRepo() *fakeGroupRepo {
	return &fakeGroupRepo{
		groups:  map[int64]*model.Group{},
		members: map[int64]map[int64]*model.GroupMember{},
	}
}

func (r *fakeGroupRepo) CreateGroup(ctx context.Context, group *model.Group) error {
	if group.ID == 0 {
		group.ID = int64(len(r.groups) + 1)
	}
	copy := *group
	r.groups[group.ID] = &copy
	return nil
}

func (r *fakeGroupRepo) GetGroupByID(ctx context.Context, id int64) (*model.Group, error) {
	group := r.groups[id]
	if group == nil {
		return nil, nil
	}
	copy := *group
	return &copy, nil
}

func (r *fakeGroupRepo) UpdateGroup(ctx context.Context, group *model.Group) error {
	copy := *group
	r.groups[group.ID] = &copy
	return nil
}

func (r *fakeGroupRepo) DeleteGroup(ctx context.Context, id int64) error {
	delete(r.groups, id)
	return nil
}

func (r *fakeGroupRepo) GetUserGroups(ctx context.Context, userID int64) ([]model.Group, error) {
	var groups []model.Group
	for groupID, members := range r.members {
		if members[userID] != nil && r.groups[groupID] != nil {
			groups = append(groups, *r.groups[groupID])
		}
	}
	return groups, nil
}

func (r *fakeGroupRepo) AddMember(ctx context.Context, member *model.GroupMember) error {
	if r.members[member.GroupID] == nil {
		r.members[member.GroupID] = map[int64]*model.GroupMember{}
	}
	copy := *member
	r.members[member.GroupID][member.UserID] = &copy
	return nil
}

func (r *fakeGroupRepo) RemoveMember(ctx context.Context, groupID, userID int64) error {
	delete(r.members[groupID], userID)
	return nil
}

func (r *fakeGroupRepo) GetMember(ctx context.Context, groupID, userID int64) (*model.GroupMember, error) {
	member := r.members[groupID][userID]
	if member == nil {
		return nil, nil
	}
	copy := *member
	return &copy, nil
}

func (r *fakeGroupRepo) GetGroupMembers(ctx context.Context, groupID int64) ([]model.GroupMember, error) {
	var members []model.GroupMember
	for _, member := range r.members[groupID] {
		members = append(members, *member)
	}
	return members, nil
}

func (r *fakeGroupRepo) UpdateMemberRole(ctx context.Context, groupID, userID int64, role string) error {
	r.members[groupID][userID].Role = role
	return nil
}

func (r *fakeGroupRepo) UpdateMuteStatus(ctx context.Context, groupID, userID int64, mutedUntil *time.Time) error {
	r.members[groupID][userID].MutedUntil = mutedUntil
	return nil
}

func (r *fakeGroupRepo) UpdateOwner(ctx context.Context, groupID, newOwnerID int64) error {
	r.groups[groupID].OwnerID = newOwnerID
	return nil
}

func (r *fakeGroupRepo) PinGroup(ctx context.Context, groupID int64, isPinned bool) error {
	r.groups[groupID].IsPinned = isPinned
	return nil
}

func TestTransferOwnerWorksWhenRedisIsDisabled(t *testing.T) {
	repo := newFakeGroupRepo()
	repo.groups[10] = &model.Group{ID: 10, Name: "team", OwnerID: 1}
	repo.members[10] = map[int64]*model.GroupMember{
		1: {GroupID: 10, UserID: 1, Role: "owner"},
		2: {GroupID: 10, UserID: 2, Role: "member"},
	}
	svc := NewGroupService(repo, nil)

	if err := svc.TransferOwner(context.Background(), 10, 1, 2); err != nil {
		t.Fatalf("TransferOwner returned error: %v", err)
	}
	if repo.groups[10].OwnerID != 2 {
		t.Fatalf("owner id = %d, want 2", repo.groups[10].OwnerID)
	}
	if repo.members[10][1].Role != "admin" {
		t.Fatalf("old owner role = %q, want admin", repo.members[10][1].Role)
	}
	if repo.members[10][2].Role != "owner" {
		t.Fatalf("new owner role = %q, want owner", repo.members[10][2].Role)
	}
}

func TestGetGroupMembersRejectsDeletedGroup(t *testing.T) {
	repo := newFakeGroupRepo()
	repo.groups[10] = &model.Group{ID: 10, Name: "team", OwnerID: 1}
	repo.members[10] = map[int64]*model.GroupMember{
		1: {GroupID: 10, UserID: 1, Role: "owner"},
		2: {GroupID: 10, UserID: 2, Role: "member"},
	}
	svc := NewGroupService(repo, nil)

	if err := svc.DeleteGroup(context.Background(), 10, 1); err != nil {
		t.Fatalf("DeleteGroup returned error: %v", err)
	}
	if _, err := svc.GetGroupMembers(context.Background(), 10); err == nil {
		t.Fatal("expected deleted group members lookup to fail")
	}
	isMember, role, err := svc.CheckMember(context.Background(), 10, 1)
	if err == nil {
		t.Fatal("expected deleted group membership check to fail")
	}
	if isMember || role != "" {
		t.Fatalf("expected no membership for deleted group, got isMember=%v role=%q", isMember, role)
	}
}

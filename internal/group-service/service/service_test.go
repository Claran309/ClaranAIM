package service

import (
	"ClaranAIM/internal/group-service/dao"
	"ClaranAIM/internal/group-service/model"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/outbox"
	"context"
	"strconv"
	"testing"
	"time"
)

type fakeGroupRepo struct {
	groups  map[int64]*model.Group
	members map[int64]map[int64]*model.GroupMember
	outbox  []outbox.Event
}

func newFakeGroupRepo() *fakeGroupRepo {
	return &fakeGroupRepo{
		groups:  map[int64]*model.Group{},
		members: map[int64]map[int64]*model.GroupMember{},
		outbox:  []outbox.Event{},
	}
}

func (r *fakeGroupRepo) WithTransaction(ctx context.Context, fn func(txRepo dao.GroupRepository) error) error {
	return fn(r)
}

func (r *fakeGroupRepo) SaveOutboxEvent(ctx context.Context, event outbox.Event) error {
	r.outbox = append(r.outbox, event)
	return nil
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

func (r *fakeGroupRepo) UpdateGroupStatus(ctx context.Context, groupID int64, status string) error {
	if r.groups[groupID] != nil {
		r.groups[groupID].Status = status
	}
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

func (r *fakeGroupRepo) AdminListGroups(ctx context.Context, keyword string, ownerID, limit, offset int64) ([]model.Group, int64, error) {
	var groups []model.Group
	for _, group := range r.groups {
		if ownerID > 0 && group.OwnerID != ownerID {
			continue
		}
		groups = append(groups, *group)
	}
	return groups, int64(len(groups)), nil
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

func TestUpdateGroupAllowsClearingAnnouncement(t *testing.T) {
	repo := newFakeGroupRepo()
	repo.groups[10] = &model.Group{ID: 10, Name: "team", OwnerID: 1, Announcement: "old notice"}
	repo.members[10] = map[int64]*model.GroupMember{
		1: {GroupID: 10, UserID: 1, Role: "owner"},
	}
	svc := NewGroupService(repo, nil)

	if err := svc.UpdateGroup(context.Background(), 10, 1, "team v2", ""); err != nil {
		t.Fatalf("UpdateGroup returned error: %v", err)
	}
	if repo.groups[10].Name != "team v2" {
		t.Fatalf("name = %q, want team v2", repo.groups[10].Name)
	}
	if repo.groups[10].Announcement != "" {
		t.Fatalf("announcement = %q, want empty string", repo.groups[10].Announcement)
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

func TestCreateGroupPublishesGroupCreatedEvent(t *testing.T) {
	repo := newFakeGroupRepo()
	svc := NewGroupService(repo, nil)

	group, err := svc.CreateGroup(context.Background(), "team", 1, []int64{2, 3})
	if err != nil {
		t.Fatalf("CreateGroup returned error: %v", err)
	}

	if len(repo.outbox) != 1 {
		t.Fatalf("outbox len = %d, want 1", len(repo.outbox))
	}
	envelope, err := repo.outbox[0].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Type != events.EventTypeGroupCreated {
		t.Fatalf("event type = %q, want %q", envelope.Type, events.EventTypeGroupCreated)
	}
	if group.ID < 1000000000 || group.ID > 9999999999 {
		t.Fatalf("group id = %d, want 10-digit public group id", group.ID)
	}
	if envelope.Key != strconv.FormatInt(group.ID, 10) {
		t.Fatalf("event key = %q, want group id %d", envelope.Key, group.ID)
	}
}

func TestJoinGroupByIDAllowsSelfJoinWithoutAdminRole(t *testing.T) {
	repo := newFakeGroupRepo()
	repo.groups[1000000001] = &model.Group{ID: 1000000001, Name: "public team", OwnerID: 1}
	repo.members[1000000001] = map[int64]*model.GroupMember{
		1: {GroupID: 1000000001, UserID: 1, Role: "owner"},
	}
	svc := NewGroupService(repo, nil)

	if err := svc.InviteMember(context.Background(), 1000000001, 2, []int64{2}); err != nil {
		t.Fatalf("self join by group id returned error: %v", err)
	}
	if repo.members[1000000001][2] == nil {
		t.Fatal("expected current user to be added as a group member")
	}
	if len(repo.outbox) != 1 {
		t.Fatalf("outbox len = %d, want one member invited event", len(repo.outbox))
	}
}

func TestInviteMemberStillRequiresAdminWhenInvitingOthers(t *testing.T) {
	repo := newFakeGroupRepo()
	repo.groups[1000000001] = &model.Group{ID: 1000000001, Name: "public team", OwnerID: 1}
	repo.members[1000000001] = map[int64]*model.GroupMember{
		1: {GroupID: 1000000001, UserID: 1, Role: "owner"},
		2: {GroupID: 1000000001, UserID: 2, Role: "member"},
	}
	svc := NewGroupService(repo, nil)

	if err := svc.InviteMember(context.Background(), 1000000001, 2, []int64{3}); err == nil {
		t.Fatal("expected non-admin invitation of another user to be denied")
	}
}

func TestCreateGroupWithIDUsesPreallocatedIDForDTMSaga(t *testing.T) {
	repo := newFakeGroupRepo()
	svc := NewGroupService(repo, nil)

	group, err := svc.CreateGroupWithID(context.Background(), 99, "dtm team", 1, []int64{2, 3})
	if err != nil {
		t.Fatalf("CreateGroupWithID returned error: %v", err)
	}

	if group.ID != 99 {
		t.Fatalf("group id = %d, want 99", group.ID)
	}
	if repo.groups[99] == nil {
		t.Fatal("expected preallocated group to be stored")
	}
	if len(repo.outbox) != 1 {
		t.Fatalf("outbox len = %d, want 1", len(repo.outbox))
	}
	envelope, err := repo.outbox[0].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Key != "99" {
		t.Fatalf("event key = %q, want 99", envelope.Key)
	}
}

func TestInviteMemberPublishesFullMemberSnapshot(t *testing.T) {
	repo := newFakeGroupRepo()
	repo.groups[10] = &model.Group{ID: 10, Name: "team", OwnerID: 1}
	repo.members[10] = map[int64]*model.GroupMember{
		1: {GroupID: 10, UserID: 1, Role: "owner"},
		2: {GroupID: 10, UserID: 2, Role: "member"},
	}
	svc := NewGroupService(repo, nil)

	if err := svc.InviteMember(context.Background(), 10, 1, []int64{3}); err != nil {
		t.Fatalf("InviteMember returned error: %v", err)
	}

	if len(repo.outbox) != 1 {
		t.Fatalf("outbox len = %d, want 1", len(repo.outbox))
	}
	envelope, err := repo.outbox[0].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Type != events.EventTypeGroupMemberInvited {
		t.Fatalf("event type = %q, want %q", envelope.Type, events.EventTypeGroupMemberInvited)
	}
	payload, err := events.DecodePayload[events.GroupMemberInvitedPayload](envelope)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.MemberIDs) != 3 {
		t.Fatalf("member snapshot len = %d, want 3: %#v", len(payload.MemberIDs), payload.MemberIDs)
	}
}

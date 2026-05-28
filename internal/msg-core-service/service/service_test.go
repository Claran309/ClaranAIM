package service

import (
	"ClaranAIM/internal/msg-core-service/dao"
	"ClaranAIM/internal/msg-core-service/model"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/pkg/events"
	"ClaranAIM/pkg/outbox"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/kitex/client/callopt"
)

type fakeMessageRepo struct {
	nextConversationID int64
	nextMessageID      int64
	nextUserStateID    int64
	conversations      map[int64]*model.Conversation
	participants       map[int64]map[int64]model.ConversationParticipant
	messages           map[int64][]model.Message
	messagesByClientID map[string]model.Message
	userStates         map[int64]map[int64]model.MessageUserState
	editRecords        []model.MessageEditRecord
	outbox             []outbox.Event
}

func newFakeMessageRepo() *fakeMessageRepo {
	return &fakeMessageRepo{
		nextConversationID: 1,
		nextMessageID:      1,
		nextUserStateID:    1,
		conversations:      map[int64]*model.Conversation{},
		participants:       map[int64]map[int64]model.ConversationParticipant{},
		messages:           map[int64][]model.Message{},
		messagesByClientID: map[string]model.Message{},
		userStates:         map[int64]map[int64]model.MessageUserState{},
		editRecords:        []model.MessageEditRecord{},
		outbox:             []outbox.Event{},
	}
}

func (r *fakeMessageRepo) WithTransaction(ctx context.Context, fn func(tx dao.MessageRepository) error) error {
	return fn(r)
}

func (r *fakeMessageRepo) SaveOutboxEvent(ctx context.Context, event outbox.Event) error {
	r.outbox = append(r.outbox, event)
	return nil
}

func (r *fakeMessageRepo) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	if conv.ID == 0 {
		conv.ID = r.nextConversationID
		r.nextConversationID++
	}
	now := time.Now()
	conv.CreatedAt = now
	conv.UpdatedAt = now
	cp := *conv
	r.conversations[conv.ID] = &cp
	return nil
}

func (r *fakeMessageRepo) GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error) {
	conv := r.conversations[id]
	if conv == nil {
		return nil, nil
	}
	cp := *conv
	return &cp, nil
}

func (r *fakeMessageRepo) UpdateConversation(ctx context.Context, conv *model.Conversation) error {
	cp := *conv
	r.conversations[conv.ID] = &cp
	return nil
}

func (r *fakeMessageRepo) AddParticipant(ctx context.Context, p *model.ConversationParticipant) error {
	if r.participants[p.ConversationID] == nil {
		r.participants[p.ConversationID] = map[int64]model.ConversationParticipant{}
	}
	if _, ok := r.participants[p.ConversationID][p.UserID]; ok {
		return nil
	}
	cp := *p
	cp.ID = int64(len(r.participants[p.ConversationID]) + 1)
	cp.JoinedAt = time.Now()
	cp.NotifyEnabled = true
	r.participants[p.ConversationID][p.UserID] = cp
	return nil
}

func (r *fakeMessageRepo) RemoveParticipant(ctx context.Context, conversationID, userID int64) error {
	delete(r.participants[conversationID], userID)
	return nil
}

func (r *fakeMessageRepo) DeleteConversation(ctx context.Context, conversationID int64) error {
	delete(r.conversations, conversationID)
	delete(r.participants, conversationID)
	return nil
}

func (r *fakeMessageRepo) GetParticipants(ctx context.Context, conversationID int64) ([]model.ConversationParticipant, error) {
	var result []model.ConversationParticipant
	for _, p := range r.participants[conversationID] {
		result = append(result, p)
	}
	return result, nil
}

func (r *fakeMessageRepo) GetUserConversations(ctx context.Context, userID int64) ([]model.ConversationParticipant, error) {
	var result []model.ConversationParticipant
	for _, byUser := range r.participants {
		if p, ok := byUser[userID]; ok {
			result = append(result, p)
		}
	}
	return result, nil
}

func (r *fakeMessageRepo) CreateMessage(ctx context.Context, msg *model.Message) error {
	if msg.ClientMsgID != nil && *msg.ClientMsgID != "" {
		if existing, ok := r.messagesByClientID[*msg.ClientMsgID]; ok {
			*msg = existing
			return nil
		}
	}
	if msg.ID == 0 {
		msg.ID = r.nextMessageID
		r.nextMessageID++
	}
	msg.CreatedAt = time.Now()
	r.messages[msg.ConversationID] = append(r.messages[msg.ConversationID], *msg)
	if msg.ClientMsgID != nil && *msg.ClientMsgID != "" {
		r.messagesByClientID[*msg.ClientMsgID] = *msg
	}
	return nil
}

func (r *fakeMessageRepo) GetMessageByClientMsgID(ctx context.Context, clientMsgID string) (*model.Message, error) {
	if msg, ok := r.messagesByClientID[clientMsgID]; ok {
		cp := msg
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeMessageRepo) GetMessageByID(ctx context.Context, messageID int64) (*model.Message, error) {
	for _, msgs := range r.messages {
		for _, msg := range msgs {
			if msg.ID == messageID {
				cp := msg
				return &cp, nil
			}
		}
	}
	return nil, nil
}

func (r *fakeMessageRepo) UpdateMessage(ctx context.Context, msg *model.Message) error {
	for convID, msgs := range r.messages {
		for i := range msgs {
			if msgs[i].ID == msg.ID {
				r.messages[convID][i] = *msg
				return nil
			}
		}
	}
	return nil
}

func (r *fakeMessageRepo) CreateEditRecord(ctx context.Context, record *model.MessageEditRecord) error {
	cp := *record
	cp.ID = int64(len(r.editRecords) + 1)
	cp.CreatedAt = time.Now()
	r.editRecords = append(r.editRecords, cp)
	return nil
}

func (r *fakeMessageRepo) UpsertMessageUserState(ctx context.Context, state *model.MessageUserState) error {
	if r.userStates[state.UserID] == nil {
		r.userStates[state.UserID] = map[int64]model.MessageUserState{}
	}
	now := time.Now()
	existing, ok := r.userStates[state.UserID][state.MessageID]
	if ok {
		if state.DeliveredAt != nil {
			existing.DeliveredAt = state.DeliveredAt
		}
		if state.ReadAt != nil {
			existing.ReadAt = state.ReadAt
		}
		if state.LocalDeletedAt != nil {
			existing.LocalDeletedAt = state.LocalDeletedAt
		}
		existing.UpdatedAt = now
		r.userStates[state.UserID][state.MessageID] = existing
		return nil
	}
	cp := *state
	cp.ID = r.nextUserStateID
	r.nextUserStateID++
	cp.CreatedAt = now
	cp.UpdatedAt = now
	r.userStates[state.UserID][state.MessageID] = cp
	return nil
}

func (r *fakeMessageRepo) MarkMessagesReadThrough(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error {
	if r.userStates[userID] == nil {
		r.userStates[userID] = map[int64]model.MessageUserState{}
	}
	for _, msg := range r.messages[conversationID] {
		if msg.ID > messageID {
			continue
		}
		state := r.userStates[userID][msg.ID]
		state.ConversationID = conversationID
		state.MessageID = msg.ID
		state.UserID = userID
		state.ReadAt = &readAt
		if state.DeliveredAt == nil {
			state.DeliveredAt = &readAt
		}
		r.userStates[userID][msg.ID] = state
	}
	return nil
}

func (r *fakeMessageRepo) MarkMessageLocalDeleted(ctx context.Context, conversationID, userID, messageID int64, deletedAt time.Time) error {
	if r.userStates[userID] == nil {
		r.userStates[userID] = map[int64]model.MessageUserState{}
	}
	state := r.userStates[userID][messageID]
	state.ConversationID = conversationID
	state.MessageID = messageID
	state.UserID = userID
	state.LocalDeletedAt = &deletedAt
	r.userStates[userID][messageID] = state
	return nil
}

func (r *fakeMessageRepo) UpdateParticipantReadCursor(ctx context.Context, conversationID, userID, messageID int64, readAt time.Time) error {
	p := r.participants[conversationID][userID]
	p.LastReadMessageID = messageID
	p.LastReadAt = &readAt
	r.participants[conversationID][userID] = p
	return nil
}

func (r *fakeMessageRepo) UpdateParticipantSettings(ctx context.Context, conversationID, userID int64, draft *string, isPinned *bool, notifyEnabled *bool) error {
	p := r.participants[conversationID][userID]
	if draft != nil {
		p.Draft = *draft
	}
	if isPinned != nil {
		p.IsPinned = *isPinned
	}
	if notifyEnabled != nil {
		p.NotifyEnabled = *notifyEnabled
	}
	r.participants[conversationID][userID] = p
	return nil
}

func (r *fakeMessageRepo) GetMessages(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.Message, error) {
	msgs := r.messages[conversationID]
	if beforeID > 0 {
		filtered := make([]model.Message, 0, len(msgs))
		for _, msg := range msgs {
			if msg.ID < beforeID {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}
	if limit > 0 && int64(len(msgs)) > limit {
		msgs = msgs[len(msgs)-int(limit):]
	}
	out := make([]model.Message, len(msgs))
	copy(out, msgs)
	return out, nil
}

func (r *fakeMessageRepo) GetMessagesForUser(ctx context.Context, conversationID, userID, limit, beforeID int64) ([]model.Message, error) {
	msgs, err := r.GetMessages(ctx, conversationID, 0, beforeID)
	if err != nil {
		return nil, err
	}
	filtered := make([]model.Message, 0, len(msgs))
	for _, msg := range msgs {
		if state, ok := r.userStates[userID][msg.ID]; ok && state.LocalDeletedAt != nil {
			continue
		}
		filtered = append(filtered, msg)
	}
	if limit > 0 && int64(len(filtered)) > limit {
		filtered = filtered[len(filtered)-int(limit):]
	}
	return filtered, nil
}

func (r *fakeMessageRepo) CountUnreadMessages(ctx context.Context, conversationID, userID, lastReadMessageID int64) (int64, error) {
	var count int64
	for _, msg := range r.messages[conversationID] {
		if msg.ID <= lastReadMessageID || msg.Status == MessageStatusRecalled {
			continue
		}
		if msg.SenderID == userID {
			continue
		}
		if state, ok := r.userStates[userID][msg.ID]; ok && state.LocalDeletedAt != nil {
			continue
		}
		count++
	}
	return count, nil
}

func (r *fakeMessageRepo) GetMessageReadStats(ctx context.Context, conversationID int64, messageIDs []int64, viewerID int64) (map[int64]dao.MessageReadStat, error) {
	wanted := map[int64]struct{}{}
	for _, id := range messageIDs {
		wanted[id] = struct{}{}
	}
	participants := r.participants[conversationID]
	stats := make(map[int64]dao.MessageReadStat, len(messageIDs))
	for _, msg := range r.messages[conversationID] {
		if _, ok := wanted[msg.ID]; !ok {
			continue
		}
		stat := dao.MessageReadStat{MessageID: msg.ID}
		for userID := range participants {
			state := r.userStates[userID][msg.ID]
			if userID == viewerID && state.ReadAt != nil {
				stat.IsReadByMe = true
			}
			if userID == msg.SenderID {
				continue
			}
			stat.RecipientCount++
			if state.ReadAt != nil {
				stat.ReadCount++
			}
		}
		stats[msg.ID] = stat
	}
	return stats, nil
}

func (r *fakeMessageRepo) SearchMessages(ctx context.Context, conversationIDs []int64, keyword string, limit int64, startAt, endAt *time.Time) ([]model.Message, error) {
	allowed := map[int64]struct{}{}
	for _, id := range conversationIDs {
		allowed[id] = struct{}{}
	}
	var result []model.Message
	for convID, msgs := range r.messages {
		if _, ok := allowed[convID]; !ok {
			continue
		}
		for _, msg := range msgs {
			if !strings.Contains(msg.Content, keyword) {
				continue
			}
			if startAt != nil && msg.CreatedAt.Before(*startAt) {
				continue
			}
			if endAt != nil && msg.CreatedAt.After(*endAt) {
				continue
			}
			result = append(result, msg)
		}
	}
	if limit > 0 && int64(len(result)) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (r *fakeMessageRepo) FindPrivateConversation(ctx context.Context, userID1, userID2 int64) (*model.Conversation, error) {
	for _, conv := range r.conversations {
		if conv.Type != "private" {
			continue
		}
		byUser := r.participants[conv.ID]
		if _, ok := byUser[userID1]; !ok {
			continue
		}
		if _, ok := byUser[userID2]; ok {
			cp := *conv
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeMessageRepo) FindGroupConversation(ctx context.Context, groupID int64) (*model.Conversation, error) {
	for _, conv := range r.conversations {
		if conv.Type == "group" && conv.GroupID == groupID {
			cp := *conv
			return &cp, nil
		}
	}
	return nil, nil
}

type fakeGroupClient struct {
	membersResp *group.GetGroupMembersResp
	membersErr  error
	groupResp   *group.GetGroupResp
	groupErr    error
}

func (c *fakeGroupClient) CreateGroup(ctx context.Context, req *group.CreateGroupReq, callOptions ...callopt.Option) (*group.CreateGroupResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) DeleteGroup(ctx context.Context, req *group.DeleteGroupReq, callOptions ...callopt.Option) (*group.DeleteGroupResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) UpdateGroup(ctx context.Context, req *group.UpdateGroupReq, callOptions ...callopt.Option) (*group.UpdateGroupResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) GetGroup(ctx context.Context, req *group.GetGroupReq, callOptions ...callopt.Option) (*group.GetGroupResp, error) {
	return c.groupResp, c.groupErr
}

func (c *fakeGroupClient) GetUserGroups(ctx context.Context, req *group.GetUserGroupsReq, callOptions ...callopt.Option) (*group.GetUserGroupsResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) InviteMember(ctx context.Context, req *group.InviteMemberReq, callOptions ...callopt.Option) (*group.InviteMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) KickMember(ctx context.Context, req *group.KickMemberReq, callOptions ...callopt.Option) (*group.KickMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) MuteMember(ctx context.Context, req *group.MuteMemberReq, callOptions ...callopt.Option) (*group.MuteMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) UnmuteMember(ctx context.Context, req *group.UnmuteMemberReq, callOptions ...callopt.Option) (*group.UnmuteMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) SetRole(ctx context.Context, req *group.SetRoleReq, callOptions ...callopt.Option) (*group.SetRoleResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) GetGroupMembers(ctx context.Context, req *group.GetGroupMembersReq, callOptions ...callopt.Option) (*group.GetGroupMembersResp, error) {
	return c.membersResp, c.membersErr
}

func (c *fakeGroupClient) CheckMember(ctx context.Context, req *group.CheckMemberReq, callOptions ...callopt.Option) (*group.CheckMemberResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) TransferOwner(ctx context.Context, req *group.TransferOwnerReq, callOptions ...callopt.Option) (*group.TransferOwnerResp, error) {
	return nil, nil
}

func (c *fakeGroupClient) PinGroup(ctx context.Context, req *group.PinGroupReq, callOptions ...callopt.Option) (*group.PinGroupResp, error) {
	return nil, nil
}

func TestCreateConversationRejectsGroupWithoutGroupID(t *testing.T) {
	svc := &messageServiceImpl{repo: newFakeMessageRepo()}

	_, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2}, 0)
	if err == nil || !strings.Contains(err.Error(), "group_id") {
		t.Fatalf("expected group_id validation error, got %v", err)
	}
}

func TestCreateConversationSyncsParticipantsForExistingGroupConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}

	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2}, 10)
	if err != nil {
		t.Fatalf("create group conversation: %v", err)
	}

	existing, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatalf("reuse group conversation: %v", err)
	}
	if existing.ID != conv.ID {
		t.Fatalf("expected existing conversation %d, got %d", conv.ID, existing.ID)
	}

	participants, err := repo.GetParticipants(context.Background(), conv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(participants) != 3 {
		t.Fatalf("expected 3 synced participants, got %d", len(participants))
	}
}

func TestCompensateGroupConversationDeletesDTMSagaConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}

	conv, err := svc.CreateGroupConversationWithID(context.Background(), 77, 99, []int64{1, 2, 3})
	if err != nil {
		t.Fatalf("CreateGroupConversationWithID returned error: %v", err)
	}
	if conv.ID != 77 {
		t.Fatalf("conversation id = %d, want 77", conv.ID)
	}

	if err := svc.CompensateGroupConversation(context.Background(), 99, 77); err != nil {
		t.Fatalf("CompensateGroupConversation returned error: %v", err)
	}
	if repo.conversations[77] != nil {
		t.Fatal("expected compensated conversation to be deleted")
	}
	if len(repo.participants[77]) != 0 {
		t.Fatalf("participants left after compensation: %#v", repo.participants[77])
	}
	if err := svc.CompensateGroupConversation(context.Background(), 99, 77); err != nil {
		t.Fatalf("second compensation should be idempotent, got %v", err)
	}
}

func TestGetHistoryRejectsNonParticipant(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	repo.messages[conv.ID] = []model.Message{{ID: 1, ConversationID: conv.ID, SenderID: 1, Content: "secret"}}

	_, err = svc.GetHistory(context.Background(), conv.ID, 99, 50, 0)
	if !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected access denied error, got %v", err)
	}
}

func TestSendMessageRejectsNonParticipant(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.SendMessage(context.Background(), conv.ID, 99, "hello", "text")
	if !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected access denied error, got %v", err)
	}
	if got := len(repo.messages[conv.ID]); got != 0 {
		t.Fatalf("expected no message to be stored, got %d", got)
	}
}

func TestSendMessageRejectsDeletedGroupEvenIfConversationParticipantRemains(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{
		repo: repo,
		groupClient: &fakeGroupClient{membersResp: &group.GetGroupMembersResp{
			Success: false,
			Msg:     "群组不存在",
		}},
	}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.SendMessage(context.Background(), conv.ID, 2, "still here?", "text")
	if !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected access denied for deleted group, got %v", err)
	}
	if got := len(repo.messages[conv.ID]); got != 0 {
		t.Fatalf("expected no fake group message to be stored, got %d", got)
	}
}

func TestGetUserConversationsSkipsDeletedGroupConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{
		repo: repo,
		groupClient: &fakeGroupClient{groupResp: &group.GetGroupResp{
			Success: false,
			Msg:     "群组不存在",
		}},
	}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatal(err)
	}
	repo.messages[conv.ID] = []model.Message{{ID: 1, ConversationID: conv.ID, SenderID: 2, Content: "old"}}

	convs, err := svc.GetUserConversations(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 {
		t.Fatalf("expected deleted group conversation placeholder, got %d", len(convs))
	}
	if !convs[0].IsDeletedGroup {
		t.Fatal("expected deleted group conversation to be marked")
	}
	if !strings.Contains(convs[0].TargetName, "已解散") {
		t.Fatalf("expected deleted group label, got %q", convs[0].TargetName)
	}
}

func TestSendMessageStoresReplyMentionsAndBroadcastType(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	root, err := svc.SendMessage(context.Background(), conv.ID, 1, "root", "text")
	if err != nil {
		t.Fatal(err)
	}

	msg, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       2,
		Content:        "reply @alice",
		MsgType:        "broadcast",
		ReplyToID:      root.ID,
		MentionUserIDs: []int64{1, 1, 0},
	})
	if err != nil {
		t.Fatalf("send ext: %v", err)
	}
	if msg.ReplyToID != root.ID {
		t.Fatalf("expected reply_to_id %d, got %d", root.ID, msg.ReplyToID)
	}
	if msg.MsgType != "broadcast" {
		t.Fatalf("expected broadcast msg type, got %q", msg.MsgType)
	}
	if len(msg.MentionUserIDs) != 1 || msg.MentionUserIDs[0] != 1 {
		t.Fatalf("expected deduped mentions [1], got %#v", msg.MentionUserIDs)
	}
}

func TestSendMessagePublishesMessageCreatedEvent(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "hello via kafka", "text")
	if err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	if len(repo.outbox) != 1 {
		t.Fatalf("outbox len = %d, want 1", len(repo.outbox))
	}
	envelope, err := repo.outbox[0].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Type != events.EventTypeMessageCreated {
		t.Fatalf("event type = %q, want %q", envelope.Type, events.EventTypeMessageCreated)
	}
	payload, err := events.DecodePayload[events.MessagePayload](envelope)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.MsgID != msg.ID || payload.ConversationID != conv.ID || len(payload.TargetUserIDs) != 2 {
		t.Fatalf("payload = %#v, want msg/conversation targets", payload)
	}
	if payload.ConversationType != "private" {
		t.Fatalf("payload conversation type = %q, want private", payload.ConversationType)
	}
	if len(payload.ParticipantIDs) != 2 {
		t.Fatalf("payload participant ids = %#v, want two participants", payload.ParticipantIDs)
	}
}

func TestSendMediaMessagePublishesAgentNativeIMEvent(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 2001}, 10)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, `[file]{"id":"3001","name":"error.png","url":"/files/3001","content_type":"image/png","size":12345}`, "file")
	if err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	if len(repo.outbox) != 2 {
		t.Fatalf("outbox len = %d, want message.created + file.uploaded", len(repo.outbox))
	}
	envelope, err := repo.outbox[1].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Type != events.EventTypeFileUploaded {
		t.Fatalf("event type = %q, want %q", envelope.Type, events.EventTypeFileUploaded)
	}
	payload, err := events.DecodePayload[events.IMEventPayload](envelope)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.MsgID != msg.ID || payload.ConversationID != conv.ID || payload.ConversationType != "group" {
		t.Fatalf("payload routing fields = %#v, want media message context", payload)
	}
	if payload.IdempotencyKey == "" {
		t.Fatal("expected idempotency key")
	}
	if len(payload.AttachmentRefs) != 1 || payload.AttachmentRefs[0].FileID != 3001 || payload.AttachmentRefs[0].Name != "error.png" {
		t.Fatalf("attachment refs = %#v, want uploaded file ref", payload.AttachmentRefs)
	}
	if len(payload.ParticipantIDs) != 3 || len(payload.Permission.VisibleUserIDs) != 3 {
		t.Fatalf("permission context missing participants: %#v", payload)
	}
}

func TestSendVoiceMessagePublishesVoiceTranscribedEventEnvelope(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.SendMessage(context.Background(), conv.ID, 1, `[voice]{"id":"88","name":"voice.webm","url":"/files/88","content_type":"audio/webm"}`, "voice")
	if err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	if len(repo.outbox) != 2 {
		t.Fatalf("outbox len = %d, want message.created + voice.transcribed", len(repo.outbox))
	}
	envelope, err := repo.outbox[1].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Type != events.EventTypeVoiceTranscribed {
		t.Fatalf("event type = %q, want %q", envelope.Type, events.EventTypeVoiceTranscribed)
	}
}

func TestApplyGroupEventPublishesAgentNativeMemberEvents(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2}, 10)
	if err != nil {
		t.Fatal(err)
	}
	repo.outbox = nil

	inviteEnvelope, err := events.NewEnvelope(events.EventTypeGroupMemberInvited, "10", events.GroupMemberInvitedPayload{
		GroupID:    10,
		OperatorID: 1,
		UserIDs:    []int64{3},
		MemberIDs:  []int64{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("NewEnvelope invite returned error: %v", err)
	}
	if err := svc.ApplyGroupEvent(context.Background(), inviteEnvelope); err != nil {
		t.Fatalf("ApplyGroupEvent invite returned error: %v", err)
	}
	if len(repo.outbox) != 1 {
		t.Fatalf("outbox len after invite = %d, want IM joined", len(repo.outbox))
	}
	joinEnvelope, err := repo.outbox[0].Envelope()
	if err != nil {
		t.Fatalf("decode join envelope: %v", err)
	}
	if joinEnvelope.Type != events.EventTypeGroupMemberJoined {
		t.Fatalf("join type = %q, want %q", joinEnvelope.Type, events.EventTypeGroupMemberJoined)
	}
	joinPayload, err := events.DecodePayload[events.IMEventPayload](joinEnvelope)
	if err != nil {
		t.Fatalf("decode join payload: %v", err)
	}
	if joinPayload.ConversationID != conv.ID || joinPayload.EventType != events.EventTypeGroupMemberJoined || joinPayload.Metadata["user_ids"] != "3" {
		t.Fatalf("join payload = %#v, want conversation-aware member join event", joinPayload)
	}
	if len(joinPayload.ParticipantIDs) != 3 {
		t.Fatalf("join participants = %#v, want full member snapshot", joinPayload.ParticipantIDs)
	}

	repo.outbox = nil
	kickEnvelope, err := events.NewEnvelope(events.EventTypeGroupMemberKicked, "10", events.GroupMemberKickedPayload{
		GroupID:    10,
		OperatorID: 1,
		UserID:     3,
		MemberIDs:  []int64{1, 2},
	})
	if err != nil {
		t.Fatalf("NewEnvelope kick returned error: %v", err)
	}
	if err := svc.ApplyGroupEvent(context.Background(), kickEnvelope); err != nil {
		t.Fatalf("ApplyGroupEvent kick returned error: %v", err)
	}
	if len(repo.outbox) != 1 {
		t.Fatalf("outbox len after kick = %d, want IM left", len(repo.outbox))
	}
	leftEnvelope, err := repo.outbox[0].Envelope()
	if err != nil {
		t.Fatalf("decode left envelope: %v", err)
	}
	if leftEnvelope.Type != events.EventTypeGroupMemberLeft {
		t.Fatalf("left type = %q, want %q", leftEnvelope.Type, events.EventTypeGroupMemberLeft)
	}
	leftPayload, err := events.DecodePayload[events.IMEventPayload](leftEnvelope)
	if err != nil {
		t.Fatalf("decode left payload: %v", err)
	}
	if leftPayload.ConversationID != conv.ID || leftPayload.EventType != events.EventTypeGroupMemberLeft || leftPayload.Metadata["user_id"] != "3" {
		t.Fatalf("left payload = %#v, want conversation-aware member left event", leftPayload)
	}
	if len(leftPayload.ParticipantIDs) != 2 {
		t.Fatalf("left participants = %#v, want remaining members", leftPayload.ParticipantIDs)
	}
}

func TestSendMessageExtReusesMessageForSameClientMsgID(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := NewMessageServiceWithPublisher(repo, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}

	first, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       1,
		Content:        "agent reply",
		MsgType:        "text",
		ClientMsgID:    "agent:evt-1:1001",
	})
	if err != nil {
		t.Fatalf("first SendMessageExt returned error: %v", err)
	}
	second, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       1,
		Content:        "agent reply should not duplicate",
		MsgType:        "text",
		ClientMsgID:    "agent:evt-1:1001",
	})
	if err != nil {
		t.Fatalf("second SendMessageExt returned error: %v", err)
	}

	if second.ID != first.ID {
		t.Fatalf("second message id = %d, want original id %d", second.ID, first.ID)
	}
	if got := len(repo.messages[conv.ID]); got != 1 {
		t.Fatalf("message count = %d, want 1", got)
	}
	if got := len(repo.outbox); got != 1 {
		t.Fatalf("outbox events = %d, want 1", got)
	}
}

func TestSendMessageExtRejectsClientMsgIDReuseAcrossSender(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := NewMessageServiceWithPublisher(repo, nil, nil)
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatalf("CreateConversation returned error: %v", err)
	}
	if _, err := svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       1,
		Content:        "first",
		MsgType:        "text",
		ClientMsgID:    "shared-key",
	}); err != nil {
		t.Fatalf("first SendMessageExt returned error: %v", err)
	}

	_, err = svc.SendMessageExt(context.Background(), SendMessageOptions{
		ConversationID: conv.ID,
		SenderID:       2,
		Content:        "second",
		MsgType:        "text",
		ClientMsgID:    "shared-key",
	})
	if err == nil || !strings.Contains(err.Error(), "client_msg_id") {
		t.Fatalf("second SendMessageExt error = %v, want client_msg_id reuse rejection", err)
	}
}

func TestMarkReadPublishesReadEventToOtherParticipants(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "read me", "text")
	if err != nil {
		t.Fatal(err)
	}
	repo.outbox = nil

	if err := svc.MarkConversationRead(context.Background(), conv.ID, 2, msg.ID); err != nil {
		t.Fatalf("MarkConversationRead returned error: %v", err)
	}

	if len(repo.outbox) != 2 {
		t.Fatalf("outbox len = %d, want legacy read + IM read", len(repo.outbox))
	}
	envelope, err := repo.outbox[0].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Type != events.EventTypeMessageRead {
		t.Fatalf("event type = %q, want %q", envelope.Type, events.EventTypeMessageRead)
	}
	payload, err := events.DecodePayload[events.MessagePayload](envelope)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload.TargetUserIDs) != 1 || payload.TargetUserIDs[0] != 1 {
		t.Fatalf("target users = %#v, want [1]", payload.TargetUserIDs)
	}
}

func TestMarkReadPublishesAgentNativeIMReadEvent(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "read me", "text")
	if err != nil {
		t.Fatal(err)
	}
	repo.outbox = nil

	if err := svc.MarkConversationRead(context.Background(), conv.ID, 2, msg.ID); err != nil {
		t.Fatalf("MarkConversationRead returned error: %v", err)
	}

	if len(repo.outbox) != 2 {
		t.Fatalf("outbox len = %d, want legacy read + IM read", len(repo.outbox))
	}
	envelope, err := repo.outbox[1].Envelope()
	if err != nil {
		t.Fatalf("outbox envelope decode failed: %v", err)
	}
	if envelope.Type != events.EventTypeIMMessageRead {
		t.Fatalf("event type = %q, want %q", envelope.Type, events.EventTypeIMMessageRead)
	}
	payload, err := events.DecodePayload[events.IMEventPayload](envelope)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.EventType != events.EventTypeMessageRead || payload.SenderID != 2 || payload.MsgID != msg.ID {
		t.Fatalf("payload = %#v, want reader and message context", payload)
	}
	if len(payload.ParticipantIDs) != 2 || payload.Permission.Scope != "private" {
		t.Fatalf("payload visibility = %#v, want private participant context", payload)
	}
}

func TestMarkReadUpdatesParticipantCursorAndUnreadCount(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(context.Background(), conv.ID, 1, "one", "text"); err != nil {
		t.Fatal(err)
	}
	second, err := svc.SendMessage(context.Background(), conv.ID, 1, "two", "text")
	if err != nil {
		t.Fatal(err)
	}

	convs, err := svc.GetUserConversations(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 || convs[0].UnreadCount != 2 {
		t.Fatalf("expected unread count 2 before read, got %#v", convs)
	}

	if err := svc.MarkConversationRead(context.Background(), conv.ID, 2, second.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	convs, err = svc.GetUserConversations(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(convs) != 1 || convs[0].UnreadCount != 0 {
		t.Fatalf("expected unread count 0 after read, got %#v", convs)
	}
}

func TestSendMessageCreatesPerUserMessageStates(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}

	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "server fact", "text")
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if msg.EditedAt != nil {
		t.Fatalf("newly sent message should not have edited_at, got %v", msg.EditedAt)
	}

	senderState, ok := repo.userStates[1][msg.ID]
	if !ok {
		t.Fatal("expected sender message_user_state to be created")
	}
	if senderState.ReadAt == nil || senderState.DeliveredAt == nil {
		t.Fatalf("expected sender message to be delivered and read, got %#v", senderState)
	}
	receiverState, ok := repo.userStates[2][msg.ID]
	if !ok {
		t.Fatal("expected receiver message_user_state to be created")
	}
	if receiverState.DeliveredAt == nil {
		t.Fatalf("expected receiver message to be marked delivered, got %#v", receiverState)
	}
	if receiverState.ReadAt != nil {
		t.Fatalf("expected receiver message to remain unread, got %#v", receiverState)
	}
}

func TestGetHistoryHydratesGroupReadReceiptCounts(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3, 4}, 10)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "read receipt", "text")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkConversationRead(context.Background(), conv.ID, 2, msg.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkConversationRead(context.Background(), conv.ID, 3, msg.ID); err != nil {
		t.Fatal(err)
	}

	history, err := svc.GetHistory(context.Background(), conv.ID, 1, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("expected one message, got %#v", history)
	}
	if history[0].RecipientCount != 3 || history[0].ReadCount != 2 {
		t.Fatalf("expected group read count 2/3, got %d/%d", history[0].ReadCount, history[0].RecipientCount)
	}
	if !history[0].IsReadByMe {
		t.Fatal("sender should see own message as read by self")
	}

	receiverHistory, err := svc.GetHistory(context.Background(), conv.ID, 4, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(receiverHistory) != 1 || receiverHistory[0].IsReadByMe {
		t.Fatalf("unread receiver should not be marked is_read_by_me, got %#v", receiverHistory)
	}
}

func TestDeleteLocalMessageHidesOnlyCurrentUsersHistory(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "visible to receiver until local delete", "text")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteLocalMessage(context.Background(), conv.ID, 2, msg.ID); err != nil {
		t.Fatalf("delete local message: %v", err)
	}

	receiverHistory, err := svc.GetHistory(context.Background(), conv.ID, 2, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(receiverHistory) != 0 {
		t.Fatalf("expected deleted message to be hidden from receiver history, got %#v", receiverHistory)
	}
	senderHistory, err := svc.GetHistory(context.Background(), conv.ID, 1, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(senderHistory) != 1 || senderHistory[0].ID != msg.ID {
		t.Fatalf("expected sender history to keep server message fact, got %#v", senderHistory)
	}
}

func TestMarkReadRejectsMessageFromAnotherConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	convA, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	convB, err := svc.CreateConversation(context.Background(), "private", []int64{1, 3}, 0)
	if err != nil {
		t.Fatal(err)
	}
	msgB, err := svc.SendMessage(context.Background(), convB.ID, 1, "wrong conversation", "text")
	if err != nil {
		t.Fatal(err)
	}

	err = svc.MarkConversationRead(context.Background(), convA.ID, 2, msgB.ID)
	if err == nil || !strings.Contains(err.Error(), "不属于当前会话") {
		t.Fatalf("expected cross-conversation read cursor rejection, got %v", err)
	}
}

func TestEditAndRecallMessageRespectSenderAndTimeLimit(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo, recallWindow: time.Hour}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "before", "text")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.EditMessage(context.Background(), msg.ID, 2, "bad"); !errors.Is(err, errConversationAccessDenied) {
		t.Fatalf("expected non-sender edit denied, got %v", err)
	}
	edited, err := svc.EditMessage(context.Background(), msg.ID, 1, "after")
	if err != nil {
		t.Fatalf("edit by sender: %v", err)
	}
	if edited.Content != "after" || !edited.IsEdited {
		t.Fatalf("expected edited message, got %#v", edited)
	}

	if err := svc.RecallMessage(context.Background(), msg.ID, 1); err != nil {
		t.Fatalf("recall by sender: %v", err)
	}
	recalled, err := repo.GetMessageByID(context.Background(), msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recalled.Status != MessageStatusRecalled || recalled.Content != "" {
		t.Fatalf("expected recalled empty message, got %#v", recalled)
	}
}

func TestEditAndRecallMessagePublishAgentNativeIMEvents(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo, recallWindow: time.Hour}
	conv, err := svc.CreateConversation(context.Background(), "group", []int64{1, 2, 3}, 10)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := svc.SendMessage(context.Background(), conv.ID, 1, "before", "text")
	if err != nil {
		t.Fatal(err)
	}
	repo.outbox = nil

	if _, err := svc.EditMessage(context.Background(), msg.ID, 1, "after"); err != nil {
		t.Fatalf("EditMessage returned error: %v", err)
	}
	if err := svc.RecallMessage(context.Background(), msg.ID, 1); err != nil {
		t.Fatalf("RecallMessage returned error: %v", err)
	}

	if len(repo.outbox) != 4 {
		t.Fatalf("outbox len = %d, want legacy edit/recalled plus IM edit/recalled", len(repo.outbox))
	}
	editEnvelope, err := repo.outbox[1].Envelope()
	if err != nil {
		t.Fatalf("decode edit IM envelope: %v", err)
	}
	if editEnvelope.Type != events.EventTypeIMMessageEdited {
		t.Fatalf("edit IM event type = %q, want %q", editEnvelope.Type, events.EventTypeIMMessageEdited)
	}
	editPayload, err := events.DecodePayload[events.IMEventPayload](editEnvelope)
	if err != nil {
		t.Fatalf("decode edit IM payload: %v", err)
	}
	if editPayload.EventType != events.EventTypeMessageEdited || editPayload.Content != "after" || len(editPayload.ParticipantIDs) != 3 {
		t.Fatalf("edit IM payload = %#v, want edited message context", editPayload)
	}

	recallEnvelope, err := repo.outbox[3].Envelope()
	if err != nil {
		t.Fatalf("decode recall IM envelope: %v", err)
	}
	if recallEnvelope.Type != events.EventTypeIMMessageRecalled {
		t.Fatalf("recall IM event type = %q, want %q", recallEnvelope.Type, events.EventTypeIMMessageRecalled)
	}
	recallPayload, err := events.DecodePayload[events.IMEventPayload](recallEnvelope)
	if err != nil {
		t.Fatalf("decode recall IM payload: %v", err)
	}
	if recallPayload.EventType != events.EventTypeMessageRecalled || recallPayload.MsgID != msg.ID || recallPayload.Content != "" {
		t.Fatalf("recall IM payload = %#v, want recalled message context", recallPayload)
	}
}

func TestSearchMessagesSupportsTimeRangeAndCurrentConversation(t *testing.T) {
	repo := newFakeMessageRepo()
	svc := &messageServiceImpl{repo: repo}
	conv, err := svc.CreateConversation(context.Background(), "private", []int64{1, 2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(context.Background(), conv.ID, 1, "alpha old", "text"); err != nil {
		t.Fatal(err)
	}
	repo.messages[conv.ID][0].CreatedAt = time.Now().Add(-48 * time.Hour)
	if _, err := svc.SendMessage(context.Background(), conv.ID, 1, "alpha new", "text"); err != nil {
		t.Fatal(err)
	}

	msgs, err := svc.SearchMessagesAdvanced(context.Background(), SearchMessagesOptions{
		UserID:          2,
		ConversationIDs: []int64{conv.ID},
		Keyword:         "alpha",
		Limit:           10,
		StartAt:         time.Now().Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Content != "alpha new" {
		t.Fatalf("expected only recent alpha message, got %#v", msgs)
	}
}

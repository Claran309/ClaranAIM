package service

import (
	"ClaranAIM/internal/user-service/model"
	"ClaranAIM/pkg/jwt"
	"ClaranAIM/pkg/password"
	"context"
	"strings"
	"testing"
)

type fakeUserRepo struct {
	user *model.User
}

func (r *fakeUserRepo) CreateUser(ctx context.Context, user *model.User) error {
	r.user = user
	return nil
}

func (r *fakeUserRepo) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	if r.user != nil && r.user.Username == username {
		return r.user, nil
	}
	return nil, nil
}

func (r *fakeUserRepo) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	if r.user != nil && r.user.ID == id {
		return r.user, nil
	}
	return nil, nil
}

func (r *fakeUserRepo) UpdateUser(ctx context.Context, user *model.User) error {
	r.user = user
	return nil
}

func (r *fakeUserRepo) BatchGetUsersByIDs(ctx context.Context, ids []int64) ([]model.User, error) {
	return nil, nil
}

func (r *fakeUserRepo) AddFriend(ctx context.Context, friend *model.Friend) error {
	return nil
}

func (r *fakeUserRepo) DeleteFriend(ctx context.Context, userID, friendID int64) error {
	return nil
}

func (r *fakeUserRepo) GetFriendList(ctx context.Context, userID int64) ([]model.Friend, error) {
	return nil, nil
}

func (r *fakeUserRepo) GetFriendByUserAndFriendID(ctx context.Context, userID, friendID int64) (*model.Friend, error) {
	return nil, nil
}

func (r *fakeUserRepo) UpdateFriendRemark(ctx context.Context, userID, friendID, groupID int64, remark string) error {
	return nil
}

func (r *fakeUserRepo) CreateFriendGroup(ctx context.Context, group *model.FriendGroup) error {
	return nil
}

func (r *fakeUserRepo) GetFriendGroups(ctx context.Context, userID int64) ([]model.FriendGroup, error) {
	return nil, nil
}

func (r *fakeUserRepo) GetFriendGroupByID(ctx context.Context, id int64) (*model.FriendGroup, error) {
	return nil, nil
}

func TestUpdateUserInfoFullUpdateAllowsClearingProfileFields(t *testing.T) {
	repo := &fakeUserRepo{user: &model.User{
		ID:        1000000001,
		Username:  "claran",
		Nickname:  "Claran",
		Email:     "old@example.com",
		Avatar:    "https://example.com/old-avatar.png",
		Cover:     "https://example.com/old-cover.png",
		Signature: "old signature",
		Bio:       "old bio",
		Location:  "old location",
		Website:   "https://old.example.com",
		Gender:    "old gender",
		Birthday:  "2000-01-01",
	}}
	svc := NewUserService(repo, nil)

	err := svc.UpdateUserInfo(context.Background(), repo.user.ID, UserProfileUpdate{
		Nickname:   "New Name",
		Email:      "",
		Phone:      "13800000000",
		Avatar:     "",
		Cover:      "",
		Signature:  "",
		Bio:        "",
		Location:   "Shanghai",
		Website:    "",
		Gender:     "",
		Birthday:   "",
		FullUpdate: true,
	})
	if err != nil {
		t.Fatalf("UpdateUserInfo returned error: %v", err)
	}

	if repo.user.Nickname != "New Name" {
		t.Fatalf("nickname = %q, want %q", repo.user.Nickname, "New Name")
	}
	if repo.user.Email != "" || repo.user.Avatar != "" || repo.user.Cover != "" || repo.user.Signature != "" || repo.user.Bio != "" || repo.user.Website != "" {
		t.Fatalf("full update should allow clearing fields, got user: %+v", repo.user)
	}
	if repo.user.Phone != "13800000000" || repo.user.Location != "Shanghai" {
		t.Fatalf("full update did not save submitted non-empty fields, got user: %+v", repo.user)
	}
}

func TestUpdateUserInfoPartialUpdateKeepsExistingFieldsOnEmptyInput(t *testing.T) {
	repo := &fakeUserRepo{user: &model.User{
		ID:        1000000002,
		Username:  "claran",
		Nickname:  "Claran",
		Email:     "old@example.com",
		Signature: "old signature",
	}}
	svc := NewUserService(repo, nil)

	err := svc.UpdateUserInfo(context.Background(), repo.user.ID, UserProfileUpdate{
		Nickname:  "New Name",
		Email:     "",
		Signature: "",
	})
	if err != nil {
		t.Fatalf("UpdateUserInfo returned error: %v", err)
	}

	if repo.user.Nickname != "New Name" {
		t.Fatalf("nickname = %q, want %q", repo.user.Nickname, "New Name")
	}
	if repo.user.Email != "old@example.com" || repo.user.Signature != "old signature" {
		t.Fatalf("partial update should keep existing fields on empty input, got user: %+v", repo.user)
	}
}

func TestSystemUserCannotPasswordLogin(t *testing.T) {
	hashed, err := password.HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	repo := &fakeUserRepo{user: &model.User{
		ID:       1000000001,
		Username: "agent_1000000001",
		Password: hashed,
		Nickname: "Agent",
		Role:     jwt.RoleUser,
		IsSystem: true,
	}}
	svc := NewUserService(repo, nil)

	_, _, err = svc.Login(context.Background(), "agent_1000000001", "secret", "test-secret", 3600, 7200)
	if err == nil || !strings.Contains(err.Error(), "系统用户") {
		t.Fatalf("Login error = %v, want system user rejection", err)
	}
}

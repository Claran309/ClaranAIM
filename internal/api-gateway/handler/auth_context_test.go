package handler

import (
	"ClaranAIM/kitex_gen/user"
	"errors"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestCurrentUserIDRejectsMissingOrInvalidContextValue(t *testing.T) {
	c := app.NewContext(0)
	if _, ok := currentUserID(c); ok {
		t.Fatal("missing userID should be rejected")
	}

	c.Set("userID", "1000000001")
	if _, ok := currentUserID(c); ok {
		t.Fatal("non-int64 userID should be rejected")
	}

	c.Set("userID", int64(0))
	if _, ok := currentUserID(c); ok {
		t.Fatal("zero userID should be rejected")
	}
}

func TestCurrentUserIDAcceptsPositiveInt64(t *testing.T) {
	c := app.NewContext(0)
	c.Set("userID", int64(1000000001))

	id, ok := currentUserID(c)
	if !ok {
		t.Fatal("positive int64 userID should be accepted")
	}
	if id != 1000000001 {
		t.Fatalf("userID = %d, want 1000000001", id)
	}
}

func TestUserInfoLookupOKRejectsNilOrFailedRPCResult(t *testing.T) {
	if userInfoLookupOK(nil, nil) {
		t.Fatal("nil user response should be rejected")
	}
	if userInfoLookupOK(&user.GetUserInfoResp{Success: true}, errors.New("rpc failed")) {
		t.Fatal("rpc error should be rejected")
	}
	if userInfoLookupOK(&user.GetUserInfoResp{Success: false}, nil) {
		t.Fatal("unsuccessful user response should be rejected")
	}
	if !userInfoLookupOK(&user.GetUserInfoResp{Success: true}, nil) {
		t.Fatal("successful user response should be accepted")
	}
}

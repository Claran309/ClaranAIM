package middleware

import (
	"ClaranAIM/pkg/jwt"
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol"
)

func TestRequireRoleAllowsAdmin(t *testing.T) {
	c := app.NewContext(0)
	c.Set("role", jwt.RoleAdmin)

	RequireRole(jwt.RoleAdmin)(context.Background(), c)

	if c.IsAborted() {
		t.Fatal("admin role should be allowed")
	}
}

func TestRequireRoleRejectsUser(t *testing.T) {
	c := app.NewContext(0)
	c.Set("role", jwt.RoleUser)
	c.Request = protocol.Request{}
	c.Response = protocol.Response{}

	RequireRole(jwt.RoleAdmin)(context.Background(), c)

	if !c.IsAborted() {
		t.Fatal("user role should be rejected")
	}
	if c.Response.StatusCode() != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", c.Response.StatusCode(), http.StatusForbidden)
	}
}

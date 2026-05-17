package jwt

import (
	"testing"
	"time"
)

func TestAccessAndRefreshTokensCarryRoleAndType(t *testing.T) {
	secret := "test-secret"
	accessToken, err := GenerateAccessToken(secret, 1000000001, "alice", RoleAdmin, 1)
	if err != nil {
		t.Fatalf("GenerateAccessToken returned error: %v", err)
	}
	refreshToken, err := GenerateRefreshToken(secret, 1000000001, "alice", RoleAdmin, 24)
	if err != nil {
		t.Fatalf("GenerateRefreshToken returned error: %v", err)
	}

	SetSecretKey(secret)
	accessClaims, err := ParseAccessToken(accessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken returned error: %v", err)
	}
	if accessClaims.Role != RoleAdmin || accessClaims.TokenType != TokenTypeAccess {
		t.Fatalf("access claims role/type = %q/%q, want %q/%q", accessClaims.Role, accessClaims.TokenType, RoleAdmin, TokenTypeAccess)
	}

	refreshClaims, err := ParseRefreshToken(refreshToken)
	if err != nil {
		t.Fatalf("ParseRefreshToken returned error: %v", err)
	}
	if refreshClaims.Role != RoleAdmin || refreshClaims.TokenType != TokenTypeRefresh {
		t.Fatalf("refresh claims role/type = %q/%q, want %q/%q", refreshClaims.Role, refreshClaims.TokenType, RoleAdmin, TokenTypeRefresh)
	}
}

func TestAccessParserRejectsRefreshToken(t *testing.T) {
	secret := "test-secret"
	refreshToken, err := GenerateRefreshToken(secret, 1000000001, "alice", RoleUser, 24)
	if err != nil {
		t.Fatal(err)
	}
	SetSecretKey(secret)

	if _, err := ParseAccessToken(refreshToken); err == nil {
		t.Fatal("expected refresh token to be rejected by access parser")
	}
}

func TestDefaultExpirationsAreNonZero(t *testing.T) {
	SetTokenExpirations(0, 0)
	if GetAccessExpirationHours() <= 0 || GetRefreshExpirationHours() <= 0 {
		t.Fatalf("expected positive defaults, got access=%d refresh=%d", GetAccessExpirationHours(), GetRefreshExpirationHours())
	}

	accessToken, err := GenerateAccessToken("secret", 1, "u", RoleUser, GetAccessExpirationHours())
	if err != nil {
		t.Fatal(err)
	}
	SetSecretKey("secret")
	claims, err := ParseAccessToken(accessToken)
	if err != nil {
		t.Fatal(err)
	}
	if time.Unix(claims.ExpiresAt, 0).Before(time.Now()) {
		t.Fatal("access token should not expire immediately")
	}
}

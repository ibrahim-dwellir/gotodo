package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gotodo/internal/auth"
)

func TestPassword_HashAndCheck(t *testing.T) {
	hash, err := auth.HashPassword("password123")
	require.NoError(t, err)
	assert.NotEqual(t, "password123", hash)
	assert.True(t, auth.CheckPassword(hash, "password123"))
	assert.False(t, auth.CheckPassword(hash, "password124"))
}

func TestTokens_RoundTrip(t *testing.T) {
	tokens, err := auth.NewTokens([]byte("secret"), time.Hour)
	require.NoError(t, err)

	tok, exp, err := tokens.Issue(42)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().Add(time.Hour), exp, 5*time.Second)

	id, err := tokens.Verify(tok)
	require.NoError(t, err)
	assert.Equal(t, int64(42), id)
}

func TestTokens_VerifyRejects(t *testing.T) {
	tokens, err := auth.NewTokens([]byte("secret"), time.Hour)
	require.NoError(t, err)

	expired, err := auth.NewTokens([]byte("secret"), -time.Minute)
	require.NoError(t, err)
	expiredTok, _, err := expired.Issue(1)
	require.NoError(t, err)

	other, err := auth.NewTokens([]byte("other"), time.Hour)
	require.NoError(t, err)
	otherTok, _, err := other.Issue(1)
	require.NoError(t, err)

	noneTok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{
		Subject: "1", Issuer: "gotodo", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	noExpTok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject: "1", Issuer: "gotodo",
	}).SignedString([]byte("secret"))
	require.NoError(t, err)

	tests := []struct {
		name  string
		token string
	}{
		{name: "expired", token: expiredTok},
		{name: "wrong secret", token: otherTok},
		{name: "alg none", token: noneTok},
		{name: "no expiry", token: noExpTok},
		{name: "garbage", token: "not-a-jwt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tokens.Verify(tc.token)
			assert.ErrorIs(t, err, auth.ErrInvalidToken)
		})
	}
}

func TestNewTokens_RejectsEmptySecret(t *testing.T) {
	_, err := auth.NewTokens(nil, time.Hour)
	assert.Error(t, err)
}

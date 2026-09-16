package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaskSensitiveInfoKeyFormats(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"openai sk-", "invalid key sk-abc1234567890", "invalid key sk-a***"},
		{"groq gsk_", "invalid key gsk_abcdefghijklmnop", "invalid key gsk_***"},
		{"hf hf_", "invalid key hf_abcdefghijklmnop", "invalid key hf_a***"},
		{"xai xai-", "invalid key xai-abcdefghijklmnop", "invalid key xai-***"},
		{"gemini AIza", "invalid key AIzaSyABCDEFGHIJKLMNOPQRSTUVWXYZ012345", "invalid key AIza***"},
		{"jwt eyJ", "invalid token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature1234567890", "invalid token eyJ***"},
		{"bearer", "Authorization: Bearer abcdefghijklmnop", "Authorization: Bearer ***"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := MaskSensitiveInfo(tc.in)
			assert.Equal(t, tc.want, out)
			assert.False(t, strings.Contains(out, "abcdefghijklmnop"))
		})
	}
}

func TestMaskSensitiveKeysDoesNotRewriteURLs(t *testing.T) {
	in := "https://api.openai.com/v1/chat type=sk-abc1234567890"
	out := MaskSensitiveKeys(in)
	require.Contains(t, out, "https://api.openai.com/v1/chat")
	require.Contains(t, out, "sk-a***")
}

func TestGetTrustQuotaDisabled(t *testing.T) {
	assert.Equal(t, 0, GetTrustQuota())
}

func TestSignMjImageDeterministic(t *testing.T) {
	SessionSecret = "abcdefghijklmnopqrstuvwxyz012345"
	sig := SignMjImage("abc123")
	require.Len(t, sig, 16)
	assert.Equal(t, sig, SignMjImage("abc123"))
	assert.NotEqual(t, sig, SignMjImage("abc124"))
}

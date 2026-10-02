package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkshopItemID(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "shared files link",
			raw:  "https://steamcommunity.com/sharedfiles/filedetails/?id=3778060647",
			want: "3778060647",
		},
		{
			name: "extra query parameters are ignored",
			raw:  "https://steamcommunity.com/sharedfiles/filedetails/?id=3778060647&searchtext=wall",
			want: "3778060647",
		},
		{
			name: "no trailing slash",
			raw:  "https://steamcommunity.com/sharedfiles/filedetails?id=123456",
			want: "123456",
		},
		{
			name: "workshop path",
			raw:  "https://steamcommunity.com/workshop/filedetails/?id=42",
			want: "42",
		},
		{
			name: "www host",
			raw:  "https://www.steamcommunity.com/sharedfiles/filedetails/?id=42",
			want: "42",
		},
		{
			name: "bare id",
			raw:  "3778060647",
			want: "3778060647",
		},
		{
			name: "surrounding whitespace",
			raw:  "  https://steamcommunity.com/sharedfiles/filedetails/?id=42  ",
			want: "42",
		},
		{name: "empty", raw: "   ", wantErr: true},
		{name: "bare id must be digits", raw: "37780abc", wantErr: true},
		{name: "javascript scheme", raw: "javascript:alert(1)", wantErr: true},
		{name: "other host", raw: "https://evil.com/sharedfiles/filedetails/?id=1", wantErr: true},
		{name: "lookalike host", raw: "https://steamcommunity.com.evil.com/sharedfiles/filedetails/?id=1", wantErr: true},
		{name: "not a workshop item", raw: "https://steamcommunity.com/profiles/76561198", wantErr: true},
		{name: "no id", raw: "https://steamcommunity.com/sharedfiles/filedetails/", wantErr: true},
		{name: "non numeric id", raw: "https://steamcommunity.com/sharedfiles/filedetails/?id=abc", wantErr: true},
		{name: "id too long", raw: "https://steamcommunity.com/sharedfiles/filedetails/?id=123456789012345678901", wantErr: true},
		{name: "scheme relative", raw: "//steamcommunity.com/sharedfiles/filedetails/?id=1", wantErr: true},
		{name: "unsupported scheme", raw: "ftp://steamcommunity.com/sharedfiles/filedetails/?id=1", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseWorkshopItemID(tc.raw)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// The resolved address is quoted into a CSS url(), so it may only ever point at
// one of Steam's own image hosts.
func TestIsSteamImageURL(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://images.steamusercontent.com/ugc/11930026390145821500/B2E419DA/", true},
		{"https://steamuserimages-a.akamaihd.net/ugc/123/abc/", true},
		{"https://cdn.akamaihd.net/steam/x.jpg", true},
		{"https://evil.com/wallpaper.jpg", false},
		{"http://localhost:8080/img.jpg", false},
		{"https://steamusercontent.com.evil.com/x.jpg", false},
		{"https://xsteamusercontent.com/x.jpg", false},
		{"javascript:alert(1)", false},
		{"data:image/svg+xml;base64,AAAA", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			assert.Equal(t, tc.want, isSteamImageURL(tc.raw))
		})
	}
}

// The type comes from the item's tags; it is what lets the console explain why a
// video wallpaper arrived as a still.
func TestIsVideoWorkshopItem(t *testing.T) {
	cases := []struct {
		name string
		tags []steamTag
		want bool
	}{
		{"video among other tags", []steamTag{{Tag: "Video"}, {Tag: "Wallpaper"}}, true},
		// Steam's casing is not guaranteed to stay put.
		{"case and padding", []steamTag{{Tag: " video "}}, true},
		{"scene is not a video", []steamTag{{Tag: "Scene"}, {Tag: "2560 x 1440"}}, false},
		{"no tags", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isVideoWorkshopItem(tc.tags))
		})
	}
}

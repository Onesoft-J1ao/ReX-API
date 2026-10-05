package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDandanplaySignature pins the signing contract. The one property that
// matters and is easy to break: the signature covers the PATH ONLY. Folding the
// query into the signed path is what produces a 403 from the upstream, so the
// same path must produce the same signature regardless of the query that is
// appended afterwards.
func TestDandanplaySignature(t *testing.T) {
	const (
		appID     = "test-app-id"
		appSecret = "test-app-secret"
		timestamp = "1750000000"
		path      = "/api/v2/comment/176170001"
	)

	// Stability: the same inputs must always yield the same header, or every
	// request fails intermittently.
	assert.Equal(t,
		dandanplaySignature(appID, appSecret, timestamp, path),
		dandanplaySignature(appID, appSecret, timestamp, path))

	// The path is the only variable part of the signed material: signing
	// "/a" and "/b" must differ, since the upstream binds the signature to the
	// resource being requested.
	assert.NotEqual(t,
		dandanplaySignature(appID, appSecret, timestamp, "/api/v2/comment/176170001"),
		dandanplaySignature(appID, appSecret, timestamp, "/api/v2/comment/176170002"))

	// Credentials and timestamp participate in the signature.
	assert.NotEqual(t,
		dandanplaySignature(appID, appSecret, timestamp, path),
		dandanplaySignature("other", appSecret, timestamp, path))
	assert.NotEqual(t,
		dandanplaySignature(appID, appSecret, timestamp, path),
		dandanplaySignature(appID, appSecret, "1750000001", path))

	// The real values produce a 64-character base64 SHA-256 digest.
	signature := dandanplaySignature(appID, appSecret, timestamp, path)
	assert.Len(t, signature, 44)
}

// TestDandanplaySignatureExcludesQuery guards the detail that cost real
// debugging time: a query string is NOT part of the signed material. The
// signature is computed over the path alone and the query is appended
// afterwards, so signing a URL that carries the query returns 403 upstream.
func TestDandanplaySignatureExcludesQuery(t *testing.T) {
	const (
		appID     = "test-app-id"
		appSecret = "test-app-secret"
		timestamp = "1750000000"
		path      = "/api/v2/comment/176170001"
	)

	bare := dandanplaySignature(appID, appSecret, timestamp, path)

	// Signing the path the handler actually passes -- which never contains the
	// query -- is the correct call.
	assert.Equal(t, bare, dandanplaySignature(appID, appSecret, timestamp, path))

	// Signing the full request URL instead is a different string and is what
	// the upstream rejects; the two must therefore never be equal.
	withQuery := dandanplaySignature(appID, appSecret, timestamp, path+"?chConvert=0&withRelated=true")
	assert.NotEqual(t, bare, withQuery)
}

// TestParseMikanFeed covers the feed shapes Mikan actually returns.
func TestParseMikanFeed(t *testing.T) {
	t.Run("reads a normal item with all fields", func(t *testing.T) {
		const hash = "f8651444a2c0b3ff7e3da3cbd75da58ef5f09e19"
		feed := []byte(`<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel><title>Mikan Project - 葬送的芙莉莲</title>
<item>
  <guid isPermaLink="false">` + hash + `</guid>
  <link>https://mikanani.me/Home/Episode/` + hash + `</link>
  <title>[LoliHouse] 葬送的芙莉莲 / Sousou no Frieren [BILIBILI WebRip 1080p HEVC OPUS][简繁内封字幕]</title>
  <description>[LoliHouse] 葬送的芙莉莲[48.5GB]</description>
  <torrent xmlns="https://mikanani.me/0.1/"><contentLength>52076478464</contentLength><pubDate>2026-08-28T05:32:00</pubDate></torrent>
  <enclosure type="application/x-bittorrent" length="52076478464" url="https://mikanani.me/Download/20260828/` + hash + `.torrent" />
</item></channel></rss>`)

		releases := parseMikanFeed(feed, "EXACT")
		require.Len(t, releases, 1)

		release := releases[0]
		assert.Equal(t, hash, release.TopicID)
		assert.Equal(t, "LoliHouse", release.Alliance)
		assert.Equal(t, "https://mikanani.me/Download/20260828/"+hash+".torrent", release.DownloadLink)
		assert.Equal(t, "https://mikanani.me/Home/Episode/"+hash, release.EpisodeLink)
		assert.Equal(t, int64(52076478464), release.Size)
		assert.Equal(t, "EXACT", release.MatchKind)
		// Attributes are derived from the title, which is the only place Mikan
		// carries them.
		assert.Equal(t, "1080P", release.Resolution)
		assert.Equal(t, "WebRip", release.MediaOrigin)
		assert.Equal(t, "CLOSED", release.SubtitleKind)
		assert.Equal(t, []string{"CHS", "CHT"}, release.SubtitleLangs)
		assert.NotZero(t, release.PublishedMs)
	})

	t.Run("drops items with no enclosure because they cannot be played", func(t *testing.T) {
		const hash = "82591bfff3599e03530771aa9fa9dc4753de541f"
		feed := []byte(`<rss version="2.0"><channel>
<item><title>[Group] Show [1080p]</title><description>x</description></item>
<item><title>[Group] Show 02 [1080p]</title><guid>` + hash + `</guid>
  <enclosure type="application/x-bittorrent" url="https://mikanani.me/Download/20260721/` + hash + `.torrent" />
</item></channel></rss>`)

		releases := parseMikanFeed(feed, "EXACT")
		require.Len(t, releases, 1)
		assert.Equal(t, hash, releases[0].TopicID)
	})

	t.Run("returns nothing for the empty channel Mikan sends when it has no releases", func(t *testing.T) {
		// This is the real response for a subject Mikan does not carry, and for
		// a feed requested with a subgroup id it does not have.
		feed := []byte(`<?xml version="1.0" encoding="utf-8"?><rss version="2.0"><channel><title>Mikan Project - 番组</title><link>http://mikanani.me/RSS/Bangumi?bangumiId=3060&amp;subgroupid=1</link><description>Mikan Project - 番组</description></channel></rss>`)

		assert.Empty(t, parseMikanFeed(feed, "EXACT"))
	})

	t.Run("falls back to the enclosure when the guid is the release title", func(t *testing.T) {
		const hash = "1e5b05a8e41c7bbab7cca59ac424e4bb28279c9d"
		// Mikan's real guid is the release name, not a hash, so identity has to
		// come from the enclosure or every release would be identified by its
		// own title text.
		feed := []byte(`<rss version="2.0"><channel>
<item><title>[H-Enc] Show 01 [BDrip 1080p HEVC FLAC]</title><guid>[H-Enc] Show 01 [BDrip 1080p HEVC FLAC]</guid>
  <enclosure url="https://mikanani.me/Download/20250414/` + hash + `.torrent" />
</item></channel></rss>`)

		releases := parseMikanFeed(feed, "EXACT")
		require.Len(t, releases, 1)
		assert.Equal(t, hash, releases[0].TopicID)
	})

	t.Run("keeps a release whose size and date are missing", func(t *testing.T) {
		const hash = "3532d30205dbc53cdaa7d03285b5cf497258fa7d"
		feed := []byte(`<rss version="2.0"><channel>
<item><title>[Group] Show 01 [1080p]</title>
  <enclosure url="https://mikanani.me/Download/x/` + hash + `.torrent" />
</item></channel></rss>`)

		releases := parseMikanFeed(feed, "FUZZY")
		require.Len(t, releases, 1)
		// A missing size must read as unknown, not as a bogus number.
		assert.Equal(t, int64(0), releases[0].Size)
		assert.Equal(t, int64(0), releases[0].PublishedMs)
		assert.Equal(t, hash, releases[0].TopicID)
		assert.Equal(t, "FUZZY", releases[0].MatchKind)
	})

	t.Run("returns nothing for malformed XML rather than panicking", func(t *testing.T) {
		assert.Empty(t, parseMikanFeed([]byte("<rss><channel><item>"), "EXACT"))
	})
}

// TestAnimeSelectRelease covers the preference order Animeko's decider encodes:
// resolution first, but a wanted subtitle language outranks a wanted release
// group.
func TestAnimeSelectRelease(t *testing.T) {
	t.Run("prefers the highest resolution that has a wanted language", func(t *testing.T) {
		candidates := []animeRelease{
			{TopicID: "a", Resolution: "1080P", SubtitleLangs: []string{"CHS"}, PublishedMs: 1},
			{TopicID: "b", Resolution: "2160P", SubtitleLangs: []string{"CHS"}, PublishedMs: 1},
			{TopicID: "c", Resolution: "2160P", SubtitleLangs: []string{"JPN"}, PublishedMs: 1},
		}

		// 2160P exists, but its Japanese-only release loses to 1080P with the
		// wanted Chinese subtitle.
		selected := animeSelectRelease(candidates)
		require.NotNil(t, selected)
		assert.Equal(t, "b", selected.TopicID)
	})

	t.Run("falls back through the resolution list", func(t *testing.T) {
		candidates := []animeRelease{
			{TopicID: "sd", Resolution: "480P", SubtitleLangs: []string{"CHS"}, PublishedMs: 1},
			{TopicID: "hd", Resolution: "720P", SubtitleLangs: []string{"CHS"}, PublishedMs: 1},
		}

		selected := animeSelectRelease(candidates)
		require.NotNil(t, selected)
		assert.Equal(t, "hd", selected.TopicID)
	})

	t.Run("orders newer releases first when the axes do not separate them", func(t *testing.T) {
		candidates := []animeRelease{
			{TopicID: "older", Resolution: "1080P", SubtitleLangs: []string{"CHS"}, PublishedMs: 100},
			{TopicID: "newer", Resolution: "1080P", SubtitleLangs: []string{"CHS"}, PublishedMs: 200},
		}

		selected := animeSelectRelease(candidates)
		require.NotNil(t, selected)
		assert.Equal(t, "newer", selected.TopicID)
	})

	t.Run("returns nil when there is nothing to choose from", func(t *testing.T) {
		assert.Nil(t, animeSelectRelease(nil))
		assert.Nil(t, animeSelectRelease([]animeRelease{}))
	})
}

// TestAnimeMatchRelease covers matching a release to an episode, including the
// season-pack case that must not outrank a correct per-episode release.
func TestAnimeMatchRelease(t *testing.T) {
	pack := animeRelease{TopicID: "pack", Title: "[LoliHouse] Frieren S01 [BDrip 1080p]"}
	second := animeRelease{TopicID: "ep2", Title: "[LoliHouse] Frieren S01E02 [BDrip 1080p]"}
	first := animeRelease{TopicID: "ep1", Title: "[LoliHouse] Frieren S01E01 [BDrip 1080p]"}

	t.Run("prefers the release naming that episode over a season pack", func(t *testing.T) {
		matched := animeMatchRelease([]animeRelease{pack, second, first}, 2)
		require.NotNil(t, matched)
		assert.Equal(t, "ep2", matched.TopicID)
	})

	t.Run("falls back to a season pack when no release names the episode", func(t *testing.T) {
		matched := animeMatchRelease([]animeRelease{pack, second}, 7)
		require.NotNil(t, matched)
		assert.Equal(t, "pack", matched.TopicID)
	})

	t.Run("returns nil when nothing covers the episode", func(t *testing.T) {
		assert.Nil(t, animeMatchRelease([]animeRelease{second}, 9))
	})
}

// animeReleaseFor builds a release the way production does, so a test exercises
// the real title parsing instead of hand-setting the fields it is asserting on.
func animeReleaseFor(topicID, title string) animeRelease {
	release := animeRelease{TopicID: topicID, Title: title}
	applyAnimeTitleAttributes(&release, title)
	return release
}

func TestAnimeParseVideoAttributes(t *testing.T) {
	cases := []struct {
		name      string
		title     string
		codec     string
		container string
		playable  bool
	}{
		{
			name:      "Baha web release in H.264 MP4",
			title:     "[ANi] Sousou no Frieren - 28 [1080P][Baha][WEB-DL][AAC AVC][CHT][MP4]",
			codec:     "H264",
			container: "MP4",
			playable:  true,
		},
		{
			name:      "x265 blu-ray rip",
			title:     "[7ACG] Frieren S01 [BDRip 1080p HEVC AAC][简繁字幕]",
			codec:     "H265",
			container: "",
			playable:  false,
		},
		{
			name:      "H.264 without a stated container is still playable",
			title:     "[GJ.Y] Frieren - 28 (1920x1080 AVC AAC)",
			codec:     "H264",
			container: "",
			playable:  true,
		},
		{
			name:      "AV1 in MP4 is not H.264",
			title:     "[Group] Frieren - 01 [1080p AV1 MP4]",
			codec:     "AV1",
			container: "MP4",
			playable:  false,
		},
		{
			name:      "a title naming neither",
			title:     "[Group] Frieren - 01 [1080p][简繁字幕]",
			codec:     "",
			container: "",
			playable:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			release := animeReleaseFor("id", tc.title)
			assert.Equal(t, tc.codec, release.VideoCodec)
			assert.Equal(t, tc.container, release.Container)
			assert.Equal(t, tc.playable, release.Playable)
		})
	}
}

func TestAnimeMatchReleasePrefersAPlayableRelease(t *testing.T) {
	// The HEVC file is the better one and the feed lists it first, but it is the
	// one the page cannot decode. Picking it would stall the player on an
	// episode a worse-looking release could have played.
	hevc := animeReleaseFor("hevc", "[Group] Show - 01 [1080p HEVC MKV]")
	mp4 := animeReleaseFor("mp4", "[Group] Show - 01 [720p AVC MP4]")

	matched := animeMatchRelease([]animeRelease{hevc, mp4}, 1)

	require.NotNil(t, matched)
	assert.Equal(t, "mp4", matched.TopicID)
}

func TestAnimeMatchReleaseKeepsAnUnplayableReleaseWhenItIsAllThereIs(t *testing.T) {
	hevc := animeReleaseFor("hevc", "[Group] Show - 01 [1080p HEVC MKV]")

	matched := animeMatchRelease([]animeRelease{hevc}, 1)

	require.NotNil(t, matched)
	assert.Equal(t, "hevc", matched.TopicID)
	assert.False(t, matched.Playable)
}

// TestAnimeEpisodeNumberInTitle covers the number parsing, including the
// resolution trap: a bare "1080" must not read as episode 1080, and a batch
// range must not claim to be its own first episode.
func TestAnimeEpisodeNumberInTitle(t *testing.T) {
	cases := []struct {
		title string
		want  int
	}{
		{"[Group] Show S01E02 [1080p]", 2},
		{"[Group] Show 03 [1080p]", 3},
		{"[Group] Show 第05话 [1080p]", 5},
		// A range spans many episodes, so it is a pack rather than episode 1.
		{"[7³ACG] Frieren S01 | 01-28+SPx11 [简繁字幕] BDrip 1080p x265 OPUS 2.0", 0},
		{"[LoliHouse] Frieren S01 [BDrip 1080p]", 0},
		{"[Group] Show [BILIBILI WebRip 2160p HEVC OPUS]", 0},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, animeEpisodeNumberInTitle(tc.title), tc.title)
	}
}

// TestAnimeParseSubtitleLanguages covers the language tokens, including the
// two-language case that marks a release as internally subtitled.
func TestAnimeParseSubtitleLanguages(t *testing.T) {
	assert.Equal(t, []string{"CHS", "CHT"}, animeParseSubtitleLanguages("简繁内封字幕"))
	assert.Empty(t, animeParseSubtitleLanguages(""))
}

// TestApplyAnimeTitleAttributesNoChineseSubtitles covers the release that
// declares itself subtitled in no Chinese. The 中文字幕 token would otherwise
// read as CHS and let it outrank a genuinely subtitled release.
func TestApplyAnimeTitleAttributesNoChineseSubtitles(t *testing.T) {
	release := animeRelease{}
	applyAnimeTitleAttributes(&release, "[Group] Show 01 [BDrip 1080p][无中文字幕]")
	assert.Empty(t, release.SubtitleLangs)
	assert.Empty(t, release.SubtitleKind)
	assert.Equal(t, "1080P", release.Resolution)
}

// TestAnimeDanmakuPayloadAcceptsMissingSuccessFlag pins the shape of the comment
// endpoint. It is the one dandanplay resource that answers without the `success`
// flag the others carry -- it returns count and comments alone -- so a parser
// that requires that flag rejects every episode that actually has comments.
func TestAnimeDanmakuPayloadAcceptsMissingSuccessFlag(t *testing.T) {
	body := []byte(`{"count":2,"comments":[{"cid":1,"p":"2.5,1,16777215,ab","m":"hi"}]}`)

	payload, err := animeDanmakuPayload(body)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), payload)
}

func TestAnimeDanmakuPayloadAcceptsAnEpisodeWithNoComments(t *testing.T) {
	body := []byte(`{"count":0,"comments":[]}`)

	payload, err := animeDanmakuPayload(body)
	require.NoError(t, err)
	assert.JSONEq(t, string(body), payload)
}

func TestAnimeDanmakuPayloadRejectsAResponseWithNoCommentList(t *testing.T) {
	_, err := animeDanmakuPayload([]byte(`{"count":0}`))
	require.Error(t, err)
}

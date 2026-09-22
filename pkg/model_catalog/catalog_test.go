package modelcatalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildEntryMapsModelsDevOntoTheConsoleVocabulary(t *testing.T) {
	var model rawModel
	model.ID = "z-ai/glm-5.3"
	model.ReleaseDate = "2026-08-14"
	model.Limit.Context = 1000000
	model.Limit.Output = 131072
	model.Modalities.Input = []string{"text", "image", "pdf"}
	model.Modalities.Output = []string{"text"}
	model.ToolCall = true
	model.Reasoning = true
	model.StructuredOutput = true
	model.OpenWeights = true

	entry := buildEntry(model)

	assert.Equal(t, 1000000, entry.ContextLength)
	assert.Equal(t, 131072, entry.MaxOutputTokens)
	// The detail view renders a YYYY-MM month, not a full date.
	assert.Equal(t, "2026-08", entry.ReleaseDate)
	assert.Equal(t, []string{"text", "image", "file"}, entry.InputModalities)
	assert.Equal(t, []string{"text"}, entry.OutputModalities)
	assert.Contains(t, entry.Capabilities, "function_calling")
	assert.Contains(t, entry.Capabilities, "reasoning")
	assert.Contains(t, entry.Capabilities, "structured_output")
	assert.Contains(t, entry.Capabilities, "vision")
}

func TestVisionFollowsImageInputNotTheAttachmentFlag(t *testing.T) {
	var attachmentOnly rawModel
	attachmentOnly.ID = "file-uploader"
	attachmentOnly.Attachment = true
	attachmentOnly.Modalities.Input = []string{"text"}

	assert.NotContains(t, buildEntry(attachmentOnly).Capabilities, "vision",
		"models.dev's attachment flag means file uploads, not image understanding")
}

func TestBuildEntryLeavesKnowledgeCutoffEmpty(t *testing.T) {
	var model rawModel
	model.ID = "glm-5.3"
	model.ReleaseDate = "2026-08-14"

	entry := buildEntry(model)

	// models.dev reports a release date and no knowledge cutoff. Filling the
	// cutoff from the release date would present a guess as a fact.
	assert.Empty(t, entry.KnowledgeCutoff)
}

func TestBuildEntryDropsModalitiesTheConsoleCannotLabel(t *testing.T) {
	var model rawModel
	model.ID = "some-model"
	model.Modalities.Input = []string{"text", "hologram"}

	entry := buildEntry(model)

	assert.Equal(t, []string{"text"}, entry.InputModalities)
}

func TestEmptyEntryIsSkipped(t *testing.T) {
	var model rawModel
	model.ID = "unknown-model"

	entry := buildEntry(model)

	assert.True(t, entry.isEmpty(),
		"a record the catalog knows nothing about must not be registered")
}

func TestNormalizeYearMonthRejectsMalformedDates(t *testing.T) {
	for _, value := range []string{"", "2026", "26-08", "2026-8", "not-a-date"} {
		assert.Empty(t, normalizeYearMonth(value), "input %q", value)
	}
	assert.Equal(t, "2026-08", normalizeYearMonth("2026-08-14"))
	assert.Equal(t, "2026-08", normalizeYearMonth(" 2026-08 "))
}

func TestBareNameStripsTheVendorPrefix(t *testing.T) {
	assert.Equal(t, "glm-5.3", bareName("z-ai/glm-5.3"))
	assert.Equal(t, "glm-5.3", bareName("glm-5.3"))
	assert.Equal(t, "gpt-4o", bareName("openai/azure/gpt-4o"))
}

func TestNormalizeModalitiesDeduplicatesAfterMapping(t *testing.T) {
	// "pdf" and "file" both map to "file"; the console must not show it twice.
	got := normalizeModalities([]string{"pdf", "file"})
	require.Equal(t, []string{"file"}, got)
}

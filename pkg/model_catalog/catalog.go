package modelcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/model_catalog_setting"
)

// catalogMaxBytes caps what a single fetch may buffer. The observed document is
// ~4.7MB; anything far past that is not the response we expect.
const catalogMaxBytes = 64 << 20

// CatalogEntry is the display-only metadata the pricing response republishes.
// Every field is optional: models.dev does not cover every model an operator
// serves, and a model it does not know keeps whatever the gateway already had.
type CatalogEntry struct {
	// ContextLength is the maximum input window in tokens.
	ContextLength int `json:"context_length,omitempty"`
	// MaxOutputTokens is the maximum tokens per response.
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`
	// KnowledgeCutoff is a YYYY-MM string. models.dev publishes the release
	// date, which is the closest signal it has to a knowledge cutoff.
	KnowledgeCutoff string `json:"knowledge_cutoff,omitempty"`
	// ReleaseDate is a YYYY-MM string.
	ReleaseDate string `json:"release_date,omitempty"`
	// InputModalities and OutputModalities use the console's vocabulary
	// (text/image/audio/video/file), not models.dev's.
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	// Capabilities uses the console's vocabulary
	// (function_calling/streaming/vision/...), not models.dev's flags.
	Capabilities []string `json:"capabilities,omitempty"`
}

// rawDocument mirrors models.dev's /api.json: a map of provider slug to an
// object holding a models map.
type rawDocument map[string]struct {
	Models map[string]rawModel `json:"models"`
}

type rawModel struct {
	ID          string `json:"id"`
	ReleaseDate string `json:"release_date"`
	Limit       struct {
		Context int `json:"context"`
		Output  int `json:"output"`
	} `json:"limit"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	ToolCall         bool `json:"tool_call"`
	Reasoning        bool `json:"reasoning"`
	StructuredOutput bool `json:"structured_output"`
	Attachment       bool `json:"attachment"`
	OpenWeights      bool `json:"open_weights"`
	Temperature      bool `json:"temperature"`
}

type cacheEntry struct {
	at      time.Time
	byModel map[string]CatalogEntry
}

var catalogCache struct {
	sync.Mutex
	entry cacheEntry
}

var (
	catalogClientOnce sync.Once
	catalogClient     *http.Client
)

func getCatalogClient(timeout time.Duration) *http.Client {
	catalogClientOnce.Do(func() {
		transport := &http.Transport{
			MaxIdleConns:          4,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   timeout,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: timeout,
		}
		if common.TLSInsecureSkipVerify {
			transport.TLSClientConfig = common.InsecureTLSConfig
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, addr)
		}
		catalogClient = &http.Client{Transport: transport}
	})
	return catalogClient
}

// Lookup returns the catalog entry for a model, or ok=false when the catalog is
// disabled, unreachable, or does not know the model. A failed fetch falls back
// to the last successful document rather than dropping the metadata the page is
// already showing.
func Lookup(modelName string) (CatalogEntry, bool) {
	setting := model_catalog_setting.GetSetting()
	if !setting.Enabled || strings.TrimSpace(modelName) == "" {
		return CatalogEntry{}, false
	}

	byModel := loadCatalog()
	if len(byModel) == 0 {
		return CatalogEntry{}, false
	}
	if entry, ok := byModel[modelName]; ok {
		return entry, true
	}
	// Upstream serves vendor-prefixed ids ("z-ai/glm-5.3") that models.dev files
	// under a bare name, so a miss on the full name retries on its tail.
	if idx := strings.LastIndex(modelName, "/"); idx >= 0 && idx+1 < len(modelName) {
		if entry, ok := byModel[modelName[idx+1:]]; ok {
			return entry, true
		}
	}
	return CatalogEntry{}, false
}

// loadCatalog returns the cached catalog, refreshing it when stale. A refresh
// failure keeps serving the previous document.
func loadCatalog() map[string]CatalogEntry {
	ttl := model_catalog_setting.GetCacheTTL()

	catalogCache.Lock()
	if time.Since(catalogCache.entry.at) < ttl && catalogCache.entry.byModel != nil {
		cached := catalogCache.entry.byModel
		catalogCache.Unlock()
		return cached
	}
	catalogCache.Unlock()

	fetched, err := fetchCatalog()
	if err != nil || len(fetched) == 0 {
		if err != nil {
			common.SysLog(fmt.Sprintf("model catalog fetch failed, keeping the previous catalog: %s", err.Error()))
		}
		catalogCache.Lock()
		stale := catalogCache.entry.byModel
		catalogCache.Unlock()
		return stale
	}

	catalogCache.Lock()
	catalogCache.entry = cacheEntry{at: time.Now(), byModel: fetched}
	catalogCache.Unlock()
	return fetched
}

func fetchCatalog() (map[string]CatalogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), model_catalog_setting.GetTimeout())
	defer cancel()

	url := model_catalog_setting.GetSourceURL()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := getCatalogClient(model_catalog_setting.GetTimeout()).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog source returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, catalogMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > catalogMaxBytes {
		return nil, errors.New("catalog document exceeds size limit")
	}
	var document rawDocument
	if err := common.Unmarshal(body, &document); err != nil {
		return nil, err
	}

	byModel := make(map[string]CatalogEntry)
	for _, provider := range document {
		for _, model := range provider.Models {
			entry := buildEntry(model)
			if entry.isEmpty() {
				continue
			}
			if model.ID != "" {
				byModel[model.ID] = entry
			}
			// A bare name is only registered when no prefixed id claimed it, so
			// a provider-qualified entry always wins over a shorter alias.
			if bare := bareName(model.ID); bare != "" {
				if _, taken := byModel[bare]; !taken {
					byModel[bare] = entry
				}
			}
		}
	}
	return byModel, nil
}

// isEmpty reports whether the catalog had nothing to say about a model. Such a
// record is skipped rather than registered, so it cannot shadow a fuller entry
// under another alias.
func (e CatalogEntry) isEmpty() bool {
	return e.ContextLength == 0 &&
		e.MaxOutputTokens == 0 &&
		e.KnowledgeCutoff == "" &&
		e.ReleaseDate == "" &&
		len(e.InputModalities) == 0 &&
		len(e.OutputModalities) == 0 &&
		len(e.Capabilities) == 0
}

func bareName(id string) string {
	if idx := strings.LastIndex(id, "/"); idx >= 0 && idx+1 < len(id) {
		return id[idx+1:]
	}
	return id
}

// buildEntry converts one models.dev record into the console's vocabulary.
func buildEntry(model rawModel) CatalogEntry {
	inputModalities := normalizeModalities(model.Modalities.Input)
	entry := CatalogEntry{
		ContextLength:    model.Limit.Context,
		MaxOutputTokens:  model.Limit.Output,
		InputModalities:  inputModalities,
		OutputModalities: normalizeModalities(model.Modalities.Output),
	}
	entry.ReleaseDate = normalizeYearMonth(model.ReleaseDate)
	// models.dev publishes a release date and no knowledge cutoff. Reusing the
	// release date would present a guess as a fact, so the cutoff stays empty
	// until a source actually reports one.
	entry.Capabilities = buildCapabilities(model, inputModalities)
	return entry
}

// normalizeModalities maps models.dev's vocabulary onto the console's. Values
// the console cannot render ("pdf") are dropped rather than passed through,
// because the detail view looks each one up in a fixed label table.
func normalizeModalities(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		mapped, ok := modalityAliases[strings.ToLower(strings.TrimSpace(value))]
		if !ok {
			continue
		}
		if _, dup := seen[mapped]; dup {
			continue
		}
		seen[mapped] = struct{}{}
		out = append(out, mapped)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

var modalityAliases = map[string]string{
	"text":  "text",
	"image": "image",
	"audio": "audio",
	"video": "video",
	"file":  "file",
	// models.dev lists documents as "pdf"; the console's label table calls the
	// same idea "file".
	"pdf": "file",
}

// buildCapabilities derives the console's capability flags from models.dev's
// booleans. Streaming is not reported by models.dev and is deliberately absent:
// claiming it would be a guess.
func buildCapabilities(model rawModel, inputModalities []string) []string {
	out := make([]string, 0, 6)
	if model.ToolCall {
		out = append(out, "function_calling", "tools")
	}
	if model.Reasoning {
		out = append(out, "reasoning")
	}
	if model.StructuredOutput {
		out = append(out, "structured_output", "json_mode")
	}
	// "Vision" means the model accepts images, which is exactly what an image
	// input modality reports. models.dev's separate `attachment` flag means the
	// model accepts file uploads at all, which is not the same claim.
	if slices.Contains(inputModalities, "image") {
		out = append(out, "vision")
	}
	if model.OpenWeights {
		out = append(out, "open_weights")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeYearMonth trims a YYYY-MM-DD date to YYYY-MM, which is the precision
// the detail view renders.
func normalizeYearMonth(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 7 {
		return ""
	}
	parts := strings.SplitN(value, "-", 3)
	if len(parts) < 2 || len(parts[0]) != 4 || len(parts[1]) != 2 {
		return ""
	}
	return parts[0] + "-" + parts[1]
}

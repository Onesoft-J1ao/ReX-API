package pricesentinel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/price_sentinel_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// upstreamMaxBytes caps what one fetch may buffer. The observed document is a
// few hundred KB; anything far past that is not the response we expect.
const upstreamMaxBytes = 16 << 20

// placeholderModelRatio is the ratio the reference site returns for a model it
// does not price. It is only a placeholder alongside a completion ratio of 1.
const placeholderModelRatio = 37.5

// nearlyEqualRatio compares two published ratios, tolerating the float noise a
// JSON round trip through the reference site can leave behind.
func nearlyEqualRatio(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// priceFields are the dimensions compared, in report order. model_ratio is the
// token price, model_price the per-request price; the rest are the multipliers
// that ride along with them.
var priceFields = []string{
	"model_ratio",
	"model_price",
	"completion_ratio",
	"cache_ratio",
}

// upstreamPricingResponse mirrors the reference site's /api/pricing envelope.
type upstreamPricingResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ModelName       string   `json:"model_name"`
		ModelRatio      float64  `json:"model_ratio"`
		ModelPrice      float64  `json:"model_price"`
		CompletionRatio float64  `json:"completion_ratio"`
		CacheRatio      *float64 `json:"cache_ratio"`
		QuotaType       int      `json:"quota_type"`
		EnableGroups    []string `json:"enable_groups"`
	} `json:"data"`
}

// Comparison is one check's outcome.
type Comparison struct {
	Compared int
	Changes  []model.PriceChange
}

// RunOnce fetches the reference site's prices, compares them against ours, and
// returns the differences. It performs no writes and sends no mail — the caller
// decides what to persist and who to tell.
func RunOnce(ctx context.Context) (*Comparison, error) {
	setting := price_sentinel_setting.GetSetting()
	url := strings.TrimSpace(setting.UpstreamPricingUrl)
	if url == "" {
		return nil, errors.New("upstream pricing url is not configured")
	}

	upstream, err := fetchUpstreamPricing(ctx, url, time.Duration(setting.TimeoutSeconds)*time.Second)
	if err != nil {
		return nil, err
	}
	if len(upstream) == 0 {
		return nil, errors.New("upstream returned no models")
	}

	ours := localPrices()
	ignored := ignoredSet(price_sentinel_setting.GetIgnoredModels())
	threshold := price_sentinel_setting.GetMinChangePercent()

	shared := make([]string, 0, len(upstream))
	for name := range upstream {
		if _, ok := ours[name]; ok {
			shared = append(shared, name)
		}
	}
	sort.Strings(shared)

	changes := make([]model.PriceChange, 0)
	for _, name := range shared {
		if _, skip := ignored[name]; skip {
			continue
		}
		changes = append(changes, compareModel(name, ours[name], upstream[name], threshold)...)
	}

	// Largest relative move first: the top of the report is what needs a
	// decision, and a long tail of float wobble should not bury it.
	sort.SliceStable(changes, func(i, j int) bool {
		return math.Abs(changes[i].Percent) > math.Abs(changes[j].Percent)
	})

	return &Comparison{Compared: len(shared), Changes: changes}, nil
}

// localPrice is one model's price as we charge it.
type localPrice struct {
	values map[string]*float64
	groups []string
}

// localPrices reads the gateway's own price tables. A field we never configured
// stays absent rather than becoming zero, because "we do not charge for this"
// and "we never set a price" are different answers.
func localPrices() map[string]localPrice {
	out := make(map[string]localPrice)

	put := func(name, field string, value float64, ok bool) {
		if !ok {
			return
		}
		entry, exists := out[name]
		if !exists {
			entry = localPrice{values: map[string]*float64{}}
		}
		v := value
		entry.values[field] = &v
		out[name] = entry
	}

	for name, value := range ratio_setting.GetModelRatioCopy() {
		put(name, "model_ratio", value, true)
	}
	for name, value := range ratio_setting.GetModelPriceCopy() {
		put(name, "model_price", value, true)
	}
	for name, value := range ratio_setting.GetCompletionRatioCopy() {
		put(name, "completion_ratio", value, true)
	}
	for name, value := range ratio_setting.GetCacheRatioCopy() {
		put(name, "cache_ratio", value, true)
	}

	for name, entry := range out {
		entry.groups = model.GetModelEnableGroups(name)
		out[name] = entry
	}
	return out
}

// compareModel returns one PriceChange per field that differs beyond threshold.
func compareModel(name string, ours localPrice, upstream upstreamPrice, threshold float64) []model.PriceChange {
	theirs := upstream.values()
	changes := make([]model.PriceChange, 0, 2)
	for _, field := range priceFields {
		ourValue, haveOurs := ours.values[field]
		theirValue, haveTheirs := theirs[field]
		if !haveOurs && !haveTheirs {
			continue
		}
		// A field only one side carries is a real difference: it means one site
		// charges for a dimension the other does not.
		if !haveOurs || !haveTheirs {
			changes = append(changes, model.PriceChange{
				ModelName: name,
				Field:     field,
				Ours:      ourValue,
				Theirs:    theirValue,
				Percent:   100,
				Groups:    ours.groups,
			})
			continue
		}
		percent := percentChange(*ourValue, *theirValue)
		if math.Abs(percent) < threshold {
			continue
		}
		changes = append(changes, model.PriceChange{
			ModelName: name,
			Field:     field,
			Ours:      ourValue,
			Theirs:    theirValue,
			Percent:   math.Round(percent*100) / 100,
			Groups:    ours.groups,
		})
	}
	return changes
}

// percentChange reports how far theirs sits from ours, as a signed percentage.
// A move away from zero from a zero base has no meaningful percentage, so it
// reports 100 rather than dividing by zero.
func percentChange(ours, theirs float64) float64 {
	if ours == 0 {
		if theirs == 0 {
			return 0
		}
		return 100
	}
	return (theirs - ours) / math.Abs(ours) * 100
}

// upstreamPrice is one model's price as the reference site charges it.
type upstreamPrice struct {
	ModelRatio      float64
	ModelPrice      float64
	CompletionRatio float64
	CacheRatio      *float64
	Groups          []string
}

// values exposes the upstream record in the same shape as ours, so the two can
// be walked by the same loop. A zero ratio means the upstream does not use that
// dimension, which is why model_ratio is only present when it is non-zero or
// the model is token-billed.
func (u upstreamPrice) values() map[string]*float64 {
	out := map[string]*float64{}
	if u.ModelRatio != 0 {
		v := u.ModelRatio
		out["model_ratio"] = &v
	}
	if u.ModelPrice != 0 {
		v := u.ModelPrice
		out["model_price"] = &v
	}
	if u.CompletionRatio != 0 {
		v := u.CompletionRatio
		out["completion_ratio"] = &v
	}
	if u.CacheRatio != nil {
		out["cache_ratio"] = u.CacheRatio
	}
	return out
}

func fetchUpstreamPricing(ctx context.Context, url string, timeout time.Duration) (map[string]upstreamPrice, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := getClient(timeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, upstreamMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > upstreamMaxBytes {
		return nil, errors.New("upstream pricing document exceeds size limit")
	}

	var decoded upstreamPricingResponse
	if err := common.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		return nil, errors.New("upstream reported failure")
	}

	out := make(map[string]upstreamPrice, len(decoded.Data))
	for _, item := range decoded.Data {
		name := strings.TrimSpace(item.ModelName)
		if name == "" {
			continue
		}
		// The reference site answers a model it does not price with a
		// placeholder ratio rather than by omitting it. Ratio sync already
		// treats that pair as untrusted; without the same rule here the sentinel
		// reports the placeholder as a real difference, and applying it prices a
		// free model at 37.5, which the square renders as $75 per million. The
		// completion ratio has to be 1 as well, because 37.5 is also a
		// legitimately published ratio (gpt-4.5-preview).
		if nearlyEqualRatio(item.ModelRatio, placeholderModelRatio) && nearlyEqualRatio(item.CompletionRatio, 1) {
			continue
		}
		out[name] = upstreamPrice{
			ModelRatio:      item.ModelRatio,
			ModelPrice:      item.ModelPrice,
			CompletionRatio: item.CompletionRatio,
			CacheRatio:      item.CacheRatio,
			Groups:          item.EnableGroups,
		}
	}
	return out, nil
}

func ignoredSet(names []string) map[string]struct{} {
	if len(names) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out[trimmed] = struct{}{}
		}
	}
	return out
}

var (
	clientOnce sync.Once
	client     *http.Client
)

func getClient(timeout time.Duration) *http.Client {
	clientOnce.Do(func() {
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
		client = &http.Client{Transport: transport}
	})
	return client
}

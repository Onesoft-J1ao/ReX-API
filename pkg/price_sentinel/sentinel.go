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
	"github.com/QuantumNous/new-api/setting/billing_setting"
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
//
// BillingMode is read even though the sentinel cannot compare expressions: the
// reference site returns a tiered model's numeric ratios alongside its
// expression, and those numbers are leftovers rather than prices. Reading the
// mode is what lets the comparison skip them.
type upstreamPricingResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ModelName       string   `json:"model_name"`
		ModelRatio      float64  `json:"model_ratio"`
		ModelPrice      float64  `json:"model_price"`
		CompletionRatio float64  `json:"completion_ratio"`
		CacheRatio      *float64 `json:"cache_ratio"`
		QuotaType       int      `json:"quota_type"`
		BillingMode     string   `json:"billing_mode"`
		EnableGroups    []string `json:"enable_groups"`
	} `json:"data"`
}

// Comparison is one check's outcome.
type Comparison struct {
	Compared int
	Changes  []model.PriceChange
	// TieredSkipped counts models left out because either side prices them with
	// an expression. The sentinel compares numeric dimensions only, so their
	// ratios are leftovers and any difference found in them would be fiction.
	TieredSkipped int
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
	// The apply path writes upstream * (1 + margin), so that is what a
	// correctly-synced model looks like here. Comparing against the raw upstream
	// number instead made every synced price read as a -16.67% difference
	// (1/1.2 - 1), and applying the report then stripped the margin back off.
	margin := price_sentinel_setting.GetMarginPercent()

	shared := make([]string, 0, len(upstream))
	for name := range upstream {
		if _, ok := ours[name]; ok {
			shared = append(shared, name)
		}
	}
	sort.Strings(shared)

	changes := make([]model.PriceChange, 0)
	tieredSkipped := 0
	for _, name := range shared {
		if _, skip := ignored[name]; skip {
			continue
		}
		ourPrice, theirPrice := ours[name], upstream[name]
		if !numericallyComparable(ourPrice, theirPrice) {
			tieredSkipped++
			continue
		}
		changes = append(changes, compareModel(name, ourPrice, theirPrice, threshold, margin)...)
	}

	// Largest relative move first: the top of the report is what needs a
	// decision, and a long tail of float wobble should not bury it.
	sort.SliceStable(changes, func(i, j int) bool {
		return math.Abs(changes[i].Percent) > math.Abs(changes[j].Percent)
	})

	return &Comparison{Compared: len(shared), Changes: changes, TieredSkipped: tieredSkipped}, nil
}

// localPrice is one model's price as we charge it.
type localPrice struct {
	values map[string]*float64
	groups []string
	// tiered marks a model this site prices with an expression. Its numeric
	// entries, if any survive in the tables, are not the price we charge.
	tiered bool
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
		entry.tiered = billing_setting.GetBillingMode(name) == billing_setting.BillingModeTieredExpr
		out[name] = entry
	}
	return out
}

// numericallyComparable reports whether both sides price this model with numbers
// the sentinel can compare.
//
// A model priced by an expression is not comparable: the reference site still
// emits its ratios, and our own tables may still hold whatever it was priced at
// before the expression took over. Neither number is the price charged, so a
// difference between them is an artifact — reporting one and applying it would
// overwrite a working expression price with a stale ratio. `ratio_sync` drops
// the same fields for the same reason, and the two paths have to agree or they
// undo each other.
func numericallyComparable(ours localPrice, theirs upstreamPrice) bool {
	return !ours.tiered && !theirs.Tiered
}

// compareModel returns one PriceChange per field that differs beyond threshold.
//
// `margin` is the markup the apply path charges, so a model already synced to
// upstream * (1 + margin) compares equal and stays out of the report. The
// reported Percent stays relative to our own price, so the number still answers
// "how far off are we" rather than "how far off is the expectation".
func compareModel(name string, ours localPrice, upstream upstreamPrice, threshold, margin float64) []model.PriceChange {
	theirs := upstream.values()
	changes := make([]model.PriceChange, 0, 2)
	for _, field := range priceFields {
		ourValue, haveOurs := ours.values[field]
		theirValue, haveTheirs := theirs[field]
		if !haveOurs && !haveTheirs {
			continue
		}
		// A field only one side carries is a difference worth reporting, because
		// it means one site charges for a dimension the other does not — but only
		// when the side that has it actually charges for it. A zero is the
		// loader's way of saying "not configured", so 0 against an absent field
		// is two ways of writing the same thing and produced most of the report.
		if !haveOurs || !haveTheirs {
			present := theirValue
			if haveOurs {
				present = ourValue
			}
			if present == nil || *present == 0 {
				continue
			}
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
		// Compare against what a correctly-synced price would be, not the raw
		// upstream number, so a healthy markup is not reported as drift. The
		// report used to flag every synced model at -16.67% (1/1.2 - 1), and
		// applying it wrote the margin back off.
		//
		// The expected value comes from the same helper the apply path writes
		// with, so the ratio fields a margin does not move are also compared
		// without one; comparing them against a marked-up expectation reported
		// every synced multiplier as drift forever.
		expected := WithMarginForField(field, *theirValue, margin)
		percent := percentChange(*ourValue, expected)
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
//
// A zero on either side has no meaningful percentage. Reporting one as 100% was
// the bulk of the report: `ours` is 0 whenever a dimension is unconfigured here
// (the loader only stores the dimensions a model actually prices), so every
// model that does not bill cache read against a model that also does not
// produced a "100% difference" row — 140 of 213 rows in one report. The caller
// decides whether a one-sided field is worth reporting; this function only
// answers the arithmetic.
func percentChange(ours, theirs float64) float64 {
	if ours == 0 || theirs == 0 {
		return 0
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
	// Tiered records that the reference site prices this model with an
	// expression, which makes the ratios above leftovers rather than prices.
	Tiered bool
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
			Tiered:          item.BillingMode == billing_setting.BillingModeTieredExpr,
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

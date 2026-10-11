package pricesentinel

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// ApplyPriceEdit writes one model's price.
//
// Only the four dimensions the sentinel reports are writable: the page exists
// to act on a reported difference, not to be a general pricing editor, and
// keeping the surface small keeps a mistyped link from touching anything else.
func ApplyPriceEdit(modelName, field string, value float64) error {
	_, err := ApplyPriceEdits([]PriceEdit{{
		ModelName: modelName,
		Field:     field,
		Value:     value,
	}})
	return err
}

// PriceEdit is one price to write.
type PriceEdit struct {
	ModelName string
	Field     string
	Value     float64
}

// BuildSyncEdits plans the writes a bulk sync of a report performs.
//
// A change whose "ours" side is absent is counted as skipped rather than
// planned. The sentinel reports those so the operator learns the upstream
// carries a model we do not, but writing a price for one would start billing
// for a model we never chose to sell.
func BuildSyncEdits(changes []model.PriceChange, marginPercent float64) (edits []PriceEdit, skipped int) {
	edits = make([]PriceEdit, 0, len(changes))
	for _, change := range changes {
		if change.Ours == nil || change.Theirs == nil {
			skipped++
			continue
		}
		edits = append(edits, PriceEdit{
			ModelName: change.ModelName,
			Field:     change.Field,
			Value:     WithMarginForField(change.Field, *change.Theirs, marginPercent),
		})
	}
	return edits, skipped
}

// ChangeIsReported reports whether a price write is one of the differences in
// the mailed report. The link authorizes acting on that report, not editing an
// arbitrary model's price.
func ChangeIsReported(changes []model.PriceChange, modelName, field string) bool {
	modelName = strings.TrimSpace(modelName)
	field = strings.TrimSpace(field)
	for _, change := range changes {
		if change.ModelName == modelName && change.Field == field {
			return true
		}
	}
	return false
}

// ratioFields are the dimensions that scale a price rather than being one.
//
// completion_ratio and cache_ratio are multipliers applied to model_ratio, so
// marking them up multiplies the margin into the customer's bill instead of
// into our price: a completion_ratio of 5 under a 20% margin would charge 6x
// the input rate, which is 20% more margin than the markup intends and turns a
// cache discount into a surcharge when cache_ratio is below 1.
var ratioFields = map[string]bool{
	"completion_ratio": true,
	"cache_ratio":      true,
}

// WithMargin returns the price to charge for an upstream price under a margin.
//
// The margin is a share of what we charge, so the upstream figure is the cost
// side of it: ours = upstream / (1 - margin). A 20% margin is therefore x1.25,
// not x1.2 — the latter is a 20% markup, which leaves only 16.67% of revenue.
//
// The result is rounded to six decimals: the upstream publishes ratios well
// inside that, and without the rounding a division leaves float noise
// (0.08 / 0.8 = 0.10000000000000002) that would be written into the price table
// and reported as a difference on the next run.
func WithMargin(upstream float64, marginPercent float64) float64 {
	if marginPercent >= 1 {
		// A margin of 100% or more has no finite price; leave the cost as-is
		// rather than dividing by zero or flipping the sign.
		return upstream
	}
	adjusted := upstream / (1 - marginPercent)
	return math.Round(adjusted*1e6) / 1e6
}

// WithMarginForField applies the margin to a price field and leaves the ratio
// fields alone.
//
// Only the dimensions that are themselves a price may carry the markup. This is
// the single place that decides, so the compare and apply paths cannot disagree
// about which fields a margin moves; when they did, a correctly synced model
// was reported as drift on the next run and applying the report rewrote the
// multiplier.
func WithMarginForField(field string, upstream float64, marginPercent float64) float64 {
	if ratioFields[field] {
		return upstream
	}
	return WithMargin(upstream, marginPercent)
}

// ApplyPriceEdits writes several prices at once.
//
// Edits are grouped by dimension and each table is read and written once, rather
// than once per model. A whole-report sync touches tens of models across a
// handful of dimensions, and writing the same table back per model would be
// both slow and racy: two concurrent applies could interleave a read and a write
// of the same table and lose one of them.
//
// Every edit is validated before anything is written, so a rejected entry leaves
// no partial change behind.
func ApplyPriceEdits(edits []PriceEdit) (int, error) {
	grouped := map[string][]PriceEdit{}
	for _, edit := range edits {
		modelName := strings.TrimSpace(edit.ModelName)
		if modelName == "" {
			return 0, errors.New("model name is empty")
		}
		if edit.Value < 0 {
			return 0, fmt.Errorf("price for %s cannot be negative", modelName)
		}
		field := strings.TrimSpace(edit.Field)
		if _, ok := priceTables[field]; !ok {
			return 0, fmt.Errorf("field %q is not editable from this page", field)
		}
		edit.ModelName = modelName
		edit.Field = field
		grouped[field] = append(grouped[field], edit)
	}

	written := 0
	for field, fieldEdits := range grouped {
		table := priceTables[field]
		current := table.read()
		for _, edit := range fieldEdits {
			current[edit.ModelName] = edit.Value
		}
		encoded, err := common.Marshal(current)
		if err != nil {
			return written, err
		}
		if err := table.load(string(encoded)); err != nil {
			return written, err
		}
		if err := model.UpdateOption(table.optionKey, string(encoded)); err != nil {
			return written, err
		}
		written += len(fieldEdits)
	}
	return written, nil
}

// priceTable describes one writable price dimension: where it is read from, the
// option row it is persisted under, and the loader that both installs it in
// memory and is the only supported way to write it.
type priceTable struct {
	optionKey string
	read      func() map[string]float64
	load      func(string) error
}

var priceTables = map[string]priceTable{
	"model_ratio": {
		optionKey: "ModelRatio",
		read:      ratio_setting.GetModelRatioCopy,
		load:      ratio_setting.UpdateModelRatioByJSONString,
	},
	"model_price": {
		optionKey: "ModelPrice",
		read:      ratio_setting.GetModelPriceCopy,
		load:      ratio_setting.UpdateModelPriceByJSONString,
	},
	"completion_ratio": {
		optionKey: "CompletionRatio",
		read:      ratio_setting.GetCompletionRatioCopy,
		load:      ratio_setting.UpdateCompletionRatioByJSONString,
	},
	"cache_ratio": {
		optionKey: "CacheRatio",
		read:      ratio_setting.GetCacheRatioCopy,
		load:      ratio_setting.UpdateCacheRatioByJSONString,
	},
}

package pricesentinel

import (
	"errors"
	"fmt"
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
//
// Each dimension is stored as one JSON document holding every model's value, so
// a partial update is not possible — the table is read, one entry replaced, and
// the whole document written back through the same loader the option store
// uses. That is what makes the change survive a restart and reach the pricing
// cache.
func ApplyPriceEdit(modelName, field string, value float64) error {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return errors.New("model name is empty")
	}
	if value < 0 {
		return errors.New("price cannot be negative")
	}

	switch strings.TrimSpace(field) {
	case "model_ratio":
		return writeTable("ModelRatio", ratio_setting.GetModelRatioCopy(), modelName, value,
			ratio_setting.UpdateModelRatioByJSONString)
	case "model_price":
		return writeTable("ModelPrice", ratio_setting.GetModelPriceCopy(), modelName, value,
			ratio_setting.UpdateModelPriceByJSONString)
	case "completion_ratio":
		return writeTable("CompletionRatio", ratio_setting.GetCompletionRatioCopy(), modelName, value,
			ratio_setting.UpdateCompletionRatioByJSONString)
	case "cache_ratio":
		return writeTable("CacheRatio", ratio_setting.GetCacheRatioCopy(), modelName, value,
			ratio_setting.UpdateCacheRatioByJSONString)
	default:
		return fmt.Errorf("field %q is not editable from this page", field)
	}
}

// writeTable replaces one entry and hands the whole table to the loader.
//
// The option row is written as well as the in-memory table: the loader alone
// would be lost on the next restart, and the option store is what the pricing
// cache reads from.
func writeTable(key string, table map[string]float64, modelName string, value float64, load func(string) error) error {
	table[modelName] = value
	encoded, err := common.Marshal(table)
	if err != nil {
		return err
	}
	if err := load(string(encoded)); err != nil {
		return err
	}
	return model.UpdateOption(key, string(encoded))
}

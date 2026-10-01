package controller

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/registrar"
	"github.com/QuantumNous/new-api/setting/registrar_setting"

	"github.com/gin-gonic/gin"
)

// Bounds shared by the settings validator and the settings UI.
const (
	maxRegistrarProviders = 20
	maxRegistrarBatch     = 1000
	maxRegistrarWorkers   = 32
)

// registrarSecretMask replaces a credential in a response body. The account
// fingerprint is the upstream login secret and the API key is a live bearer
// token, so neither is returned unless the operator explicitly asks to reveal
// it, and that request is audited.
const registrarSecretMask = "********"

type registrarChannelStatus struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	MultiKey   bool   `json:"multi_key"`
	TotalKeys  int    `json:"total_keys"`
	UsableKeys int    `json:"usable_keys"`
	Group      string `json:"group"`
}

type registrarProviderView struct {
	registrar_setting.Provider
	AccountCount int                    `json:"account_count"`
	Channel      registrarChannelStatus `json:"channel"`
}

// GetRegistrarProviders reports each configured provider with the live state the
// admin page needs: how many accounts are archived and how many keys its channel
// can still use.
func GetRegistrarProviders(c *gin.Context) {
	setting := registrar_setting.GetSetting()
	views := make([]registrarProviderView, 0, len(setting.Providers))
	for _, provider := range setting.Providers {
		view := registrarProviderView{Provider: provider}
		if count, err := registrar.CountAccounts(provider.ID); err == nil {
			view.AccountCount = count
		}
		if channel, err := model.GetChannelById(provider.ChannelID, true); err == nil && channel != nil {
			view.Channel = registrarChannelStatus{
				ID:         channel.Id,
				Name:       channel.Name,
				MultiKey:   channel.ChannelInfo.IsMultiKey,
				TotalKeys:  len(channel.GetKeys()),
				UsableKeys: registrar.UsableKeyCount(channel),
				Group:      channel.Group,
			}
		}
		views = append(views, view)
	}
	common.ApiSuccess(c, gin.H{
		"enabled":          setting.Enabled,
		"interval_minutes": setting.IntervalMinutes,
		"providers":        views,
	})
}

type registrarRunRequest struct {
	Count int `json:"count"`
}

// RunRegistrarProvider queues one manual registration run. The request only
// picks the provider and how many accounts to register; the script path and the
// destination channel come from server-side settings, and the task handler
// clamps the count to the provider's batch cap.
func RunRegistrarProvider(c *gin.Context) {
	provider, ok := registrar_setting.GetProvider(c.Param("id"))
	if !ok {
		common.ApiError(c, fmt.Errorf("unknown registrar provider %q", c.Param("id")))
		return
	}
	request := registrarRunRequest{}
	if raw, err := c.GetRawData(); err == nil && len(raw) > 0 {
		if err := common.Unmarshal(raw, &request); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if request.Count <= 0 {
		common.ApiError(c, fmt.Errorf("count must be positive"))
		return
	}
	if provider.MaxBatch > 0 && request.Count > provider.MaxBatch {
		request.Count = provider.MaxBatch
	}

	task, created, err := service.EnqueueSystemTask(model.SystemTaskTypeRegistrarRun, registrarRunPayload{
		ProviderID: provider.ID,
		Count:      request.Count,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if created {
		recordManageAudit(c, "registrar.run", map[string]any{
			"provider_id": provider.ID,
			"count":       request.Count,
			"task_id":     task.TaskID,
		})
	}
	common.ApiSuccess(c, gin.H{
		"created": created,
		"task":    task.ToResponse(),
	})
}

// ListRegistrarAccounts returns archived accounts. Credentials are masked unless
// the operator passes reveal=1, and revealing them is audited because the
// fingerprint is the upstream account's password.
func ListRegistrarAccounts(c *gin.Context) {
	providerID := c.Query("provider")
	if _, ok := registrar_setting.GetProvider(providerID); !ok {
		common.ApiError(c, fmt.Errorf("unknown registrar provider %q", providerID))
		return
	}
	accounts, err := registrar.LoadAccounts(providerID)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	reveal := c.Query("reveal") == "1"
	if reveal {
		recordManageAudit(c, "registrar.accounts.reveal", map[string]any{
			"provider_id": providerID,
			"count":       len(accounts),
		})
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}
	start := min((page-1)*pageSize, len(accounts))
	end := min(start+pageSize, len(accounts))

	items := make([]gin.H, 0, end-start)
	for _, account := range accounts[start:end] {
		item := gin.H{
			"username":   account.Username,
			"created_at": account.CreatedAt,
			"channel_id": account.ChannelID,
		}
		if reveal {
			item["fingerprint"] = account.Fingerprint
			item["api_key"] = account.APIKey
		} else {
			item["fingerprint"] = registrarSecretMask
			item["api_key"] = registrarSecretMask
		}
		items = append(items, item)
	}

	common.ApiSuccess(c, gin.H{
		"items":     items,
		"total":     len(accounts),
		"page":      page,
		"page_size": pageSize,
		"masked":    !reveal,
	})
}

// ExportRegistrarAccounts downloads the provider's raw account log. The file
// holds fingerprints, so the download is audited.
func ExportRegistrarAccounts(c *gin.Context) {
	providerID := c.Query("provider")
	if _, ok := registrar_setting.GetProvider(providerID); !ok {
		common.ApiError(c, fmt.Errorf("unknown registrar provider %q", providerID))
		return
	}
	path, err := registrar.AccountsPath(providerID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "registrar.accounts.export", map[string]any{"provider_id": providerID})
	c.FileAttachment(path, providerID+"-accounts.jsonl")
}

type registrarSettingsRequest struct {
	Enabled         bool                         `json:"enabled"`
	IntervalMinutes int                          `json:"interval_minutes"`
	Providers       []registrar_setting.Provider `json:"providers"`
}

// UpdateRegistrarProviders validates and persists the registrar settings. It is
// a dedicated endpoint rather than the generic option writer because the
// provider list carries a server-side script path and a destination channel id,
// and those are worth checking before they are stored.
func UpdateRegistrarProviders(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	request := registrarSettingsRequest{}
	if err := common.Unmarshal(raw, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := validateRegistrarSettings(request); err != nil {
		common.ApiError(c, err)
		return
	}

	providersJSON, err := common.Marshal(request.Providers)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// Each key is written through the option table, whose update path applies the
	// value back onto the registered config, so the running instance picks the
	// new settings up without a restart.
	for key, value := range map[string]string{
		"registrar_setting.enabled":          strconv.FormatBool(request.Enabled),
		"registrar_setting.interval_minutes": strconv.Itoa(request.IntervalMinutes),
		"registrar_setting.providers":        string(providersJSON),
	} {
		if err := model.UpdateOption(key, value); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	recordManageAudit(c, "registrar.settings.update", map[string]any{
		"providers": len(request.Providers),
		"enabled":   request.Enabled,
	})
	common.ApiSuccess(c, nil)
}

func validateRegistrarSettings(request registrarSettingsRequest) error {
	if request.IntervalMinutes < 1 || request.IntervalMinutes > 24*60 {
		return fmt.Errorf("interval_minutes must be between 1 and 1440")
	}
	if len(request.Providers) > maxRegistrarProviders {
		return fmt.Errorf("at most %d registrar providers are supported", maxRegistrarProviders)
	}
	seen := make(map[string]struct{}, len(request.Providers))
	for _, provider := range request.Providers {
		if !isValidRegistrarProviderID(provider.ID) {
			return fmt.Errorf("invalid provider id %q", provider.ID)
		}
		if _, ok := seen[provider.ID]; ok {
			return fmt.Errorf("duplicate provider id %q", provider.ID)
		}
		seen[provider.ID] = struct{}{}
		if strings.TrimSpace(provider.Label) == "" || len(provider.Label) > 64 {
			return fmt.Errorf("provider %s needs a label of at most 64 characters", provider.ID)
		}
		if !filepath.IsAbs(strings.TrimSpace(provider.ScriptPath)) {
			return fmt.Errorf("provider %s needs an absolute script path", provider.ID)
		}
		if provider.ChannelID <= 0 {
			return fmt.Errorf("provider %s needs a destination channel", provider.ID)
		}
		if provider.MinKeys < 0 || provider.TargetKeys < 0 {
			return fmt.Errorf("provider %s key counts must not be negative", provider.ID)
		}
		if provider.TargetKeys < provider.MinKeys {
			return fmt.Errorf("provider %s target keys must not be below its floor", provider.ID)
		}
		if provider.MaxBatch < 1 || provider.MaxBatch > maxRegistrarBatch {
			return fmt.Errorf("provider %s batch cap must be between 1 and %d", provider.ID, maxRegistrarBatch)
		}
		if provider.Workers < 1 || provider.Workers > maxRegistrarWorkers {
			return fmt.Errorf("provider %s workers must be between 1 and %d", provider.ID, maxRegistrarWorkers)
		}
	}
	return nil
}

func isValidRegistrarProviderID(id string) bool {
	if id == "" || len(id) > 32 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

type registrarImportRequest struct {
	ProviderID string `json:"provider_id"`
	// Content is a dahl_keys.txt-style body: one "username|fingerprint|key" per
	// line. It is the archive of accounts registered before the gateway managed
	// them, so only the credential record is imported; the keys are already on
	// the channel.
	Content string `json:"content"`
}

// ImportRegistrarAccounts folds an existing account archive into the store. It
// deduplicates by API key against what is already archived, so re-running an
// import is safe.
func ImportRegistrarAccounts(c *gin.Context) {
	raw, err := c.GetRawData()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	request := registrarImportRequest{}
	if err := common.Unmarshal(raw, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if _, ok := registrar_setting.GetProvider(request.ProviderID); !ok {
		common.ApiError(c, fmt.Errorf("unknown registrar provider %q", request.ProviderID))
		return
	}

	existing, err := registrar.LoadAccounts(request.ProviderID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	seen := make(map[string]struct{}, len(existing)+64)
	for _, account := range existing {
		seen[account.APIKey] = struct{}{}
	}

	var toAdd []registrar.Account
	skipped := 0
	for line := range strings.SplitSeq(request.Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			skipped++
			continue
		}
		username := strings.TrimSpace(parts[0])
		fingerprint := strings.TrimSpace(parts[1])
		apiKey := strings.TrimSpace(parts[2])
		if fingerprint == "" || apiKey == "" {
			skipped++
			continue
		}
		if _, ok := seen[apiKey]; ok {
			skipped++
			continue
		}
		seen[apiKey] = struct{}{}
		toAdd = append(toAdd, registrar.Account{
			Username:    username,
			Fingerprint: fingerprint,
			APIKey:      apiKey,
			CreatedAt:   common.GetTimestamp(),
		})
	}

	if err := registrar.AppendAccounts(request.ProviderID, toAdd); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "registrar.accounts.import", map[string]any{
		"provider_id": request.ProviderID,
		"imported":    len(toAdd),
		"skipped":     skipped,
	})
	common.ApiSuccess(c, gin.H{"imported": len(toAdd), "skipped": skipped})
}

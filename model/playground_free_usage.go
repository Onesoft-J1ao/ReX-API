package model

import (
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PlaygroundFreeScopeFingerprint = "fingerprint"
	PlaygroundFreeScopeIP          = "ip"
)

// PlaygroundFreeUsage records how much of the free playground allowance one
// visitor has spent. It is keyed by scope rather than by account on purpose: the
// allowance belongs to the person, so a second account opened in the same
// browser must not start a fresh one.
type PlaygroundFreeUsage struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Scope       string `json:"scope" gorm:"type:varchar(16);not null;uniqueIndex:idx_playground_free_scope"`
	ScopeKey    string `json:"scope_key" gorm:"type:varchar(191);not null;uniqueIndex:idx_playground_free_scope"`
	Generations int    `json:"generations" gorm:"not null"`
	Tokens      int64  `json:"tokens" gorm:"bigint;not null"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint"`
}

func (PlaygroundFreeUsage) TableName() string {
	return "playground_free_usages"
}

// playgroundFreeScopes lists the counters one visitor is measured against. The
// fingerprint is the primary one; the caller IP is counted too, so clearing
// browser storage does not restore the allowance.
func playgroundFreeScopes(fingerprint, ip string) [][2]string {
	scopes := make([][2]string, 0, 2)
	if fingerprint != "" {
		scopes = append(scopes, [2]string{PlaygroundFreeScopeFingerprint, fingerprint})
	}
	if ip != "" {
		scopes = append(scopes, [2]string{PlaygroundFreeScopeIP, ip})
	}
	return scopes
}

// PlaygroundFreeAllowanceAvailable reports whether this visitor may still be
// served from the free allowance. Whichever budget runs out first ends it, and
// an exhausted counter on any scope ends it for that visitor.
func PlaygroundFreeAllowanceAvailable(fingerprint, ip string) (bool, error) {
	setting := operation_setting.GetPlaygroundSetting()
	if !setting.Enabled || (setting.FreeGenerations <= 0 && setting.FreeTokens <= 0) {
		return false, nil
	}
	scopes := playgroundFreeScopes(fingerprint, ip)
	if len(scopes) == 0 {
		return false, nil
	}
	if !setting.DedupeByIP {
		scopes = scopes[:1]
	}

	for _, scope := range scopes {
		var row PlaygroundFreeUsage
		err := DB.Where("scope = ? AND scope_key = ?", scope[0], scope[1]).First(&row).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return false, err
		}
		if setting.FreeGenerations > 0 && row.Generations >= setting.FreeGenerations {
			return false, nil
		}
		if setting.FreeTokens > 0 && row.Tokens >= setting.FreeTokens {
			return false, nil
		}
	}
	return true, nil
}

// RecordPlaygroundFreeUsage books one covered request against every scope the
// visitor is measured against. Counters are incremented in SQL so concurrent
// requests cannot lose an update.
func RecordPlaygroundFreeUsage(fingerprint, ip string, tokens int64) error {
	scopes := playgroundFreeScopes(fingerprint, ip)
	setting := operation_setting.GetPlaygroundSetting()
	if !setting.DedupeByIP {
		scopes = scopes[:1]
	}
	if len(scopes) == 0 || tokens < 0 {
		return nil
	}

	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, scope := range scopes {
			row := PlaygroundFreeUsage{
				Scope:       scope[0],
				ScopeKey:    scope[1],
				Generations: 1,
				Tokens:      tokens,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "scope"}, {Name: "scope_key"}},
				DoUpdates: clause.Assignments(map[string]any{
					"generations": gorm.Expr("generations + ?", 1),
					"tokens":      gorm.Expr("tokens + ?", tokens),
					"updated_at":  now,
				}),
			}).Create(&row).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
}

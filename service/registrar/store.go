package registrar

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// Account is one registered upstream account. Fingerprint is the account's only
// credential — it is the password the upstream site issues once and cannot
// recover, so it is persisted next to the key that was cut from it. It must
// never reach a log line.
type Account struct {
	Username    string `json:"username"`
	Fingerprint string `json:"fingerprint"`
	APIKey      string `json:"api_key"`
	CreatedAt   int64  `json:"created_at"`
	ChannelID   int    `json:"channel_id"`
}

// baseDir returns the root the registrar writes under. The gateway container
// runs with its working directory on the /data volume, so the default relative
// path survives an image swap; REGISTRAR_DATA_DIR overrides it.
func baseDir() string {
	if dir := strings.TrimSpace(os.Getenv("REGISTRAR_DATA_DIR")); dir != "" {
		return dir
	}
	return "registrar"
}

func providerDir(providerID string) (string, error) {
	if providerID == "" || strings.ContainsAny(providerID, `/\`) || providerID == "." || providerID == ".." {
		return "", fmt.Errorf("invalid provider id %q", providerID)
	}
	return filepath.Join(baseDir(), providerID), nil
}

// AccountsPath is the per-provider append-only account log.
func AccountsPath(providerID string) (string, error) {
	dir, err := providerDir(providerID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "accounts.jsonl"), nil
}

var storeMu sync.Mutex

// AppendAccounts writes accounts to the provider log and flushes them to disk.
// Registration is incremental on purpose: an interrupted run keeps whatever it
// already registered instead of losing the whole batch.
func AppendAccounts(providerID string, accounts []Account) error {
	if len(accounts) == 0 {
		return nil
	}
	path, err := AccountsPath(providerID)
	if err != nil {
		return err
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, account := range accounts {
		line, err := common.Marshal(account)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return file.Sync()
}

// LoadAccounts reads the provider log. A torn or unparsable line is skipped so
// one bad write cannot hide every account behind it.
func LoadAccounts(providerID string) ([]Account, error) {
	path, err := AccountsPath(providerID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var accounts []Account
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var account Account
		if err := common.UnmarshalJsonStr(line, &account); err != nil {
			continue
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func CountAccounts(providerID string) (int, error) {
	accounts, err := LoadAccounts(providerID)
	if err != nil {
		return 0, err
	}
	return len(accounts), nil
}

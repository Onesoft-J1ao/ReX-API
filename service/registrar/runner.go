package registrar

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/registrar_setting"
)

// RunResult reports what one registration run produced. Registered accounts are
// already persisted by the time it is returned.
type RunResult struct {
	ProviderID string    `json:"provider_id"`
	Requested  int       `json:"requested"`
	Registered []Account `json:"-"`
	Added      int       `json:"added"`
}

var (
	runLocksMu sync.Mutex
	runLocks   = map[string]*sync.Mutex{}
)

// providerLock serializes runs of one provider inside this process. Cross-node
// serialization is the system task lease's job.
func providerLock(providerID string) *sync.Mutex {
	runLocksMu.Lock()
	defer runLocksMu.Unlock()
	lock, ok := runLocks[providerID]
	if !ok {
		lock = &sync.Mutex{}
		runLocks[providerID] = lock
	}
	return lock
}

func pythonBin() string {
	if bin := strings.TrimSpace(os.Getenv("REGISTRAR_PYTHON")); bin != "" {
		return bin
	}
	return "python3"
}

// RunProvider registers up to count accounts for a provider and appends their
// keys to the bound channel. Accounts are persisted as they arrive, so a run
// killed part-way still keeps what it registered.
func RunProvider(ctx context.Context, provider registrar_setting.Provider, count int, progress func(processed, total int)) (RunResult, error) {
	result := RunResult{ProviderID: provider.ID, Requested: count}
	if count <= 0 {
		return result, nil
	}
	scriptPath := strings.TrimSpace(provider.ScriptPath)
	if scriptPath == "" || !filepath.IsAbs(scriptPath) {
		return result, fmt.Errorf("provider %s has no absolute script path", provider.ID)
	}

	lock := providerLock(provider.ID)
	lock.Lock()
	defer lock.Unlock()

	accounts, err := register(ctx, provider, scriptPath, count, progress)
	result.Registered = accounts
	if err != nil {
		return result, err
	}
	if len(accounts) == 0 {
		return result, fmt.Errorf("provider %s registered no accounts", provider.ID)
	}

	keys := make([]string, 0, len(accounts))
	for _, account := range accounts {
		keys = append(keys, account.APIKey)
	}
	added, err := AppendKeysToChannel(provider.ChannelID, keys)
	if err != nil {
		// The accounts are saved, so nothing is lost; report that the keys did not
		// reach the channel instead of pretending the run fully succeeded.
		return result, fmt.Errorf("registered %d accounts but appending keys to channel %d failed: %w", len(accounts), provider.ChannelID, err)
	}
	result.Added = added
	return result, nil
}

// register runs the provider script and consumes one JSON account per stdout
// line. Each account is written to the store immediately; stderr is captured as
// the diagnostic tail for a failed run.
func register(ctx context.Context, provider registrar_setting.Provider, scriptPath string, count int, progress func(processed, total int)) ([]Account, error) {
	workers := provider.Workers
	if workers < 1 {
		workers = 1
	}
	if workers > 32 {
		workers = 32
	}
	args := []string{scriptPath, "--count", strconv.Itoa(count), "--workers", strconv.Itoa(workers), "--json"}
	if !provider.UseProxy {
		args = append(args, "--no-proxy")
	}

	cmd := exec.CommandContext(ctx, pythonBin(), args...)
	cmd.Dir = filepath.Dir(scriptPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr := newCappedBuffer(8192)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var accounts []Account
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		account, ok := parseAccountLine(scanner.Text())
		if !ok {
			continue
		}
		account.ChannelID = provider.ChannelID
		account.CreatedAt = common.GetTimestamp()
		if err := AppendAccounts(provider.ID, []Account{account}); err != nil {
			return accounts, fmt.Errorf("persisting a registered account failed: %w", err)
		}
		accounts = append(accounts, account)
		if progress != nil {
			progress(len(accounts), count)
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if scanErr != nil {
		return accounts, scanErr
	}
	if waitErr != nil {
		return accounts, fmt.Errorf("registrar script failed: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return accounts, nil
}

// parseAccountLine reads one account from a runner stdout line. The runner
// writes its own progress logs to stderr, so anything on stdout that is not an
// account object with a key is ignored rather than treated as a failure.
func parseAccountLine(line string) (Account, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Account{}, false
	}
	var account Account
	if err := common.UnmarshalJsonStr(line, &account); err != nil {
		return Account{}, false
	}
	if account.APIKey == "" {
		return Account{}, false
	}
	return account, true
}

// cappedBuffer keeps the first limit bytes written to it. The registrar script's
// stderr is its own log stream; without a cap a runaway script could grow the
// captured text without bound while the run is killed.
type cappedBuffer struct {
	limit int
	data  []byte
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{limit: limit}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if remaining := b.limit - len(b.data); remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		b.data = append(b.data, p[:remaining]...)
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	return string(b.data)
}

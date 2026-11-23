package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SeniorAlfred/macron-agent/pkg/app"
	"github.com/SeniorAlfred/macron-agent/pkg/config"
)

const (
	settingExecutionMode = "execution_mode"
	envExecutionMode     = config.EnvExecutionMode
)

var executionModeMu sync.Mutex

func normalizeExecutionMode(v string) (string, bool) {
	switch m := strings.ToLower(strings.TrimSpace(v)); m {
	case "local", "sandbox", "best-effort":
		return m, true
	default:
		return "", false
	}
}

func executionModeFlagSet(fs *flag.FlagSet) bool {
	if fs == nil {
		return false
	}
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "execution-mode" {
			set = true
		}
	})
	return set
}

// applySavedExecutionMode loads the persisted execution mode from the settings
// store only when MACRON_EXECUTION_MODE is not explicitly set in the environment.
func applySavedExecutionMode(cfg *config.Config, getSetting func(string) (string, bool)) {
	if cfg == nil || getSetting == nil {
		return
	}
	if strings.TrimSpace(os.Getenv(envExecutionMode)) != "" {
		return
	}
	saved, ok := getSetting(settingExecutionMode)
	if !ok {
		return
	}
	if mode, valid := normalizeExecutionMode(saved); valid {
		cfg.ExecutionMode = mode
	}
}

func switchExecutionMode(ctx context.Context, a *app.App, save func(context.Context, string, string) error, mode string) (string, error) {
	if a == nil || a.Machine == nil {
		return "", errors.New("execution machine is not initialized")
	}

	executionModeMu.Lock()
	defer executionModeMu.Unlock()

	prev := a.Machine.ExecutionMode()
	note, err := a.Machine.SetExecutionMode(mode)
	if err != nil {
		return "", err
	}
	active := a.Machine.ExecutionMode()
	if save != nil {
		sctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = save(sctx, settingExecutionMode, active)
		cancel()
		if err != nil {
			if prev != active {
				_, _ = a.Machine.SetExecutionMode(prev)
			}
			return "", fmt.Errorf("save execution mode: %w", err)
		}
	}
	if a.Config != nil {
		a.Config.ExecutionMode = active
	}
	return note, nil
}

func (t *terminal) ExecutionMode() string {
	if t.app != nil && t.app.Machine != nil {
		return t.app.Machine.ExecutionMode()
	}
	if t.st != nil {
		if v, _ := t.st.Setting(settingExecutionMode); v != "" {
			return v
		}
	}
	return "best-effort"
}

func (t *terminal) Sandboxed() bool {
	return t.app != nil && t.app.Machine != nil && t.app.Machine.Sandboxed()
}

func (t *terminal) executionStatus() string {
	mode := t.ExecutionMode()
	if t.Sandboxed() {
		return mode + " (sandbox active)"
	}
	return mode + " (unsandboxed)"
}

func (t *terminal) SetExecutionMode(mode string) (string, error) {
	var save func(context.Context, string, string) error
	if t.st != nil {
		save = t.st.SetSetting
	}
	return switchExecutionMode(context.Background(), t.app, save, mode)
}

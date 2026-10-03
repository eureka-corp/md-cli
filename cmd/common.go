package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eureka-corp/md-cli/internal/config"
	"github.com/eureka-corp/md-cli/internal/mdshare"
)

// baseURL resolves MD_URL, then the saved config, then the default.
func baseURL(cfg config.Config) string {
	for _, u := range []string{os.Getenv("MD_URL"), cfg.BaseURL} {
		if u != "" {
			return strings.TrimRight(u, "/")
		}
	}
	return mdshare.DefaultBaseURL
}

// newClient resolves the token from MD_TOKEN, the saved config, then orion's.
func newClient() (*mdshare.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	token := os.Getenv("MD_TOKEN")
	if token == "" {
		token = cfg.Token
	}
	if token == "" {
		token = config.OrionToken()
	}
	if token == "" {
		return nil, fmt.Errorf("no upload token — run 'md setup' or set MD_TOKEN")
	}
	return &mdshare.Client{BaseURL: baseURL(cfg), Token: token}, nil
}

func shareState() mdshare.State {
	return mdshare.State{
		Path:     filepath.Join(config.Dir(), "shares.json"),
		Fallback: filepath.Join(config.OrionDir(), "md-shares.json"),
	}
}

func shareIDArg(arg string) (string, error) {
	id := mdshare.ShareID(arg)
	if id == "" {
		return "", fmt.Errorf("invalid share %q", arg)
	}
	return id, nil
}

// formatExpiry renders an expiry as an absolute local time plus a relative hint.
func formatExpiry(t *time.Time, now time.Time) string {
	if t == nil {
		return "never"
	}
	return fmt.Sprintf("%s (%s)", t.Local().Format("2006-01-02 15:04"), relative(t.Sub(now)))
}

func relative(d time.Duration) string {
	if d < 0 {
		return "expired"
	}
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("in %dd", int(d.Round(24*time.Hour)/(24*time.Hour)))
	case d >= 2*time.Hour:
		return fmt.Sprintf("in %dh", int(d.Round(time.Hour)/time.Hour))
	default:
		return fmt.Sprintf("in %dm", int(d.Round(time.Minute)/time.Minute))
	}
}

// Package updatecheck tells the user when a newer release exists, at most once a day.
package updatecheck

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/blang/semver/v4"
	"github.com/creativeprojects/go-selfupdate"
)

const cacheTTL = 24 * time.Hour

type cache struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checkedAt"`
}

// Newer returns the latest release version when it is newer than current.
func Newer(ctx context.Context, current, cacheDir, owner, repo string) (string, bool) {
	cur, err := semver.ParseTolerant(current)
	if err != nil {
		return "", false
	}
	cacheFile := filepath.Join(cacheDir, "version-check.json")

	var c cache
	if data, err := os.ReadFile(cacheFile); err == nil && json.Unmarshal(data, &c) == nil &&
		time.Since(c.CheckedAt) < cacheTTL {
		return compare(c.Latest, cur)
	}

	latest, err := fetchLatest(ctx, owner, repo)
	if err != nil {
		return "", false
	}
	if data, err := json.Marshal(cache{Latest: latest, CheckedAt: time.Now()}); err == nil {
		_ = os.MkdirAll(cacheDir, 0o700)
		_ = os.WriteFile(cacheFile, data, 0o600)
	}
	return compare(latest, cur)
}

func compare(latest string, current semver.Version) (string, bool) {
	v, err := semver.ParseTolerant(latest)
	if err != nil || !v.GT(current) {
		return "", false
	}
	return v.String(), true
}

func fetchLatest(ctx context.Context, owner, repo string) (string, error) {
	updater, err := selfupdate.NewUpdater(selfupdate.Config{})
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	release, found, err := updater.DetectLatest(ctx, selfupdate.NewRepositorySlug(owner, repo))
	if err != nil || !found {
		return "", err
	}
	return release.Version(), nil
}

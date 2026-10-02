package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	t.Setenv("MD_CONFIG_DIR", t.TempDir())
	if c, err := Load(); err != nil || c != (Config{}) {
		t.Fatalf("Load on a missing file = %+v, %v", c, err)
	}
	if err := Save(Config{Token: "t", BaseURL: "u"}); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(); err != nil || c.Token != "t" || c.BaseURL != "u" {
		t.Fatalf("Load = %+v, %v", c, err)
	}
}

func TestOrionToken(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("ORION_MD_TOKEN", "")
	if got := OrionToken(); got != "" {
		t.Fatalf("no orion config: %q", got)
	}
	if err := os.MkdirAll(filepath.Join(xdg, "orion"), 0o700); err != nil {
		t.Fatal(err)
	}
	yml := "cloudflare:\n  token: x\nmd:\n  upload_token: mdr_abc\n  base_url: https://md.erk.im\n"
	if err := os.WriteFile(filepath.Join(xdg, "orion", "config.yaml"), []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := OrionToken(); got != "mdr_abc" {
		t.Fatalf("OrionToken = %q", got)
	}
	t.Setenv("ORION_MD_TOKEN", "env")
	if got := OrionToken(); got != "env" {
		t.Fatalf("env should win: %q", got)
	}
}

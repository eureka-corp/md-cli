package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeServer records requests and answers like the mdreader API.
type fakeServer struct {
	mu       sync.Mutex
	requests []string
	bodies   []map[string]any
	gone     map[string]bool
}

func (f *fakeServer) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, body)

		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Missing or invalid upload token","code":"unauthorized"}`))
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/shares/")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/shares":
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/shares":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new1","url":"https://md.test/s/new1","expiresAt":"2030-01-01T00:00:00Z","ttl":86400,"fileCount":1}`))
		case r.Method == http.MethodPut && f.gone[id]:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Share not found or expired","code":"not_found"}`))
		case r.Method == http.MethodPut || r.Method == http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"` + id + `","url":"https://md.test/s/` + id + `","title":"T","expiresAt":null,"ttl":null,"fileCount":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/me/shares":
			_, _ = w.Write([]byte(`{"shares":[{"id":"a","url":"https://md.test/s/a","title":"Design docs","fileCount":3,"expiresAt":null}]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
}

func setup(t *testing.T) (*fakeServer, string) {
	t.Helper()
	f := &fakeServer{gone: map[string]bool{}}
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	t.Setenv("MD_CONFIG_DIR", filepath.Join(dir, "md"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "xdg")) // isolates the orion fallback
	t.Setenv("MD_URL", srv.URL)
	t.Setenv("MD_TOKEN", "tok")
	t.Setenv("ORION_MD_TOKEN", "")

	doc := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(doc, []byte("# hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f, doc
}

func run(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func TestShareCreatesAndPrintsOnlyTheLink(t *testing.T) {
	f, doc := setup(t)
	out, errOut, err := run(t, "", "share", doc, "--ttl", "24h")
	if err != nil {
		t.Fatal(err)
	}
	if out != "https://md.test/s/new1\n" {
		t.Fatalf("stdout = %q", out)
	}
	if !strings.Contains(errOut, "Shared 1 file(s)") {
		t.Fatalf("stderr = %q", errOut)
	}
	if f.bodies[0]["ttl"] != float64(86400) {
		t.Fatalf("body = %v", f.bodies[0])
	}
}

func TestShareSlidesNeverAndTitle(t *testing.T) {
	f, doc := setup(t)
	out, _, err := run(t, "", "share", doc, "--ttl", "never", "--slides", "--title", "Deck")
	if err != nil {
		t.Fatal(err)
	}
	if out != "https://md.test/s/new1?present\n" {
		t.Fatalf("stdout = %q", out)
	}
	if f.bodies[0]["keep"] != true || f.requests[1] != "PATCH /api/shares/new1" || f.bodies[1]["title"] != "Deck" {
		t.Fatalf("requests = %v bodies = %v", f.requests, f.bodies)
	}
}

func TestShareUpdateReusesTheLink(t *testing.T) {
	f, doc := setup(t)
	if _, _, err := run(t, "", "share", doc, "--ttl", "1h"); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := run(t, "", "share", doc, "--update")
	if err != nil {
		t.Fatal(err)
	}
	if f.requests[1] != "PUT /api/shares/new1" || out != "https://md.test/s/new1\n" {
		t.Fatalf("requests = %v stdout = %q", f.requests, out)
	}
	if _, ok := f.bodies[1]["ttl"]; ok {
		t.Fatalf("update without --ttl must keep the expiry: %v", f.bodies[1])
	}
	if !strings.Contains(errOut, "Updated") {
		t.Fatalf("stderr = %q", errOut)
	}

	// A share that is gone falls back to a new one.
	f.gone["new1"] = true
	if _, errOut, err = run(t, "", "share", doc, "--update", "--ttl", "1h"); err != nil {
		t.Fatal(err)
	}
	if f.requests[3] != "POST /api/shares" || !strings.Contains(errOut, "no longer exists") {
		t.Fatalf("requests = %v stderr = %q", f.requests, errOut)
	}
}

func TestShareFromStdin(t *testing.T) {
	f, _ := setup(t)
	if _, _, err := run(t, "# piped", "share", "-", "--name", "notes.md"); err != nil {
		t.Fatal(err)
	}
	files := f.bodies[0]["files"].([]any)
	if file := files[0].(map[string]any); file["path"] != "notes.md" || file["content"] != "# piped" {
		t.Fatalf("file = %v", file)
	}
}

func TestManageCommands(t *testing.T) {
	f, _ := setup(t)
	out, _, err := run(t, "", "list")
	if err != nil || !strings.Contains(out, "Design docs") || !strings.Contains(out, "never") {
		t.Fatalf("list = %q, %v", out, err)
	}
	out, _, err = run(t, "", "list", "--json")
	var shares []map[string]any
	if err != nil || json.Unmarshal([]byte(out), &shares) != nil || len(shares) != 1 {
		t.Fatalf("list --json = %q, %v", out, err)
	}

	cases := []struct {
		args []string
		body string
	}{
		{[]string{"extend", "https://md.test/s/abc"}, `{"extend":604800}`},
		{[]string{"extend", "abc", "2h", "--from-now"}, `{"ttl":7200}`},
		{[]string{"keep", "abc"}, `{"keep":true}`},
		{[]string{"rename", "abc", "New title"}, `{"title":"New title"}`},
		{[]string{"rename", "abc"}, `{"title":null}`},
	}
	for _, tc := range cases {
		n := len(f.requests)
		if _, _, err := run(t, "", tc.args...); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		b, _ := json.Marshal(f.bodies[n])
		if f.requests[n] != "PATCH /api/shares/abc" || string(b) != tc.body {
			t.Errorf("%v sent %s %s, want %s", tc.args, f.requests[n], b, tc.body)
		}
	}

	if _, _, err := run(t, "", "unshare", "abc", "https://md.test/s/def"); err != nil {
		t.Fatal(err)
	}
	if got := f.requests[len(f.requests)-2:]; got[0] != "DELETE /api/shares/abc" || got[1] != "DELETE /api/shares/def" {
		t.Fatalf("unshare requests = %v", got)
	}
}

func TestSetup(t *testing.T) {
	setup(t)
	t.Setenv("MD_TOKEN", "")
	if _, _, err := run(t, "", "setup", "--token", "wrong"); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("bad token err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("MD_CONFIG_DIR"), "config.json")); err == nil {
		t.Fatal("a rejected token was saved")
	}

	if _, _, err := run(t, "", "setup", "--token", "tok"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(os.Getenv("MD_CONFIG_DIR"), "config.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config file = %v, %v", info, err)
	}
	// The saved token is used once MD_TOKEN is unset.
	if _, _, err := run(t, "", "list"); err != nil {
		t.Fatal(err)
	}
}

func TestMissingToken(t *testing.T) {
	setup(t)
	t.Setenv("MD_TOKEN", "")
	if _, _, err := run(t, "", "list"); err == nil || !strings.Contains(err.Error(), "md setup") {
		t.Fatalf("err = %v", err)
	}
}

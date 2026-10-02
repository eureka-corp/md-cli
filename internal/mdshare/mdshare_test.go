package mdshare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func paths(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	sort.Strings(out)
	return out
}

func TestCollectDirectory(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"docs/intro.md":               "# intro",
		"docs/guide/setup.MDX":        "# setup",
		"docs/guide/old.markdown":     "# old",
		"docs/guide/diagram.png":      "png",
		"docs/.git/HEAD.md":           "no",
		"docs/node_modules/pkg/a.md":  "no",
		"docs/.obsidian/workspace.md": "no",
		"other/outside.md":            "# outside",
	})
	if err := os.Symlink(filepath.Join(tmp, "other", "outside.md"), filepath.Join(tmp, "docs", "link.md")); err != nil {
		t.Fatal(err)
	}

	// Trailing slash must still yield "docs" as the first segment.
	files, err := Collect([]string{filepath.Join(tmp, "docs") + "/"}, nil, "stdin.md")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"docs/guide/old.markdown", "docs/guide/setup.MDX", "docs/intro.md"}
	if got := paths(files); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", got, want)
	}
}

func TestCollectFilesAndStdin(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{"a/readme.md": "# a"})

	files, err := Collect(
		[]string{filepath.Join(tmp, "a", "readme.md"), "-"},
		strings.NewReader("# piped"), "notes.md",
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := paths(files); strings.Join(got, ",") != "notes.md,readme.md" {
		t.Fatalf("paths = %v", got)
	}
	if files[1].Content != "# piped" {
		t.Fatalf("stdin content = %q", files[1].Content)
	}
}

func TestCollectErrors(t *testing.T) {
	tmp := t.TempDir()
	writeTree(t, tmp, map[string]string{
		"a/readme.md": "# a",
		"b/readme.md": "# b",
		"notes.txt":   "text",
		"empty/x.txt": "text",
		"bad.md":      "\xff\xfe",
		"big/big.md":  strings.Repeat("x", MaxBytes+1),
	})

	cases := map[string]struct {
		args      []string
		stdinName string
		wantErr   string
	}{
		"duplicate":      {[]string{filepath.Join(tmp, "a", "readme.md"), filepath.Join(tmp, "b", "readme.md")}, "stdin.md", "duplicate path"},
		"not markdown":   {[]string{filepath.Join(tmp, "notes.txt")}, "stdin.md", "not a markdown file"},
		"empty dir":      {[]string{filepath.Join(tmp, "empty")}, "stdin.md", "no markdown files"},
		"missing":        {[]string{filepath.Join(tmp, "nope.md")}, "stdin.md", "no such file"},
		"invalid utf8":   {[]string{filepath.Join(tmp, "bad.md")}, "stdin.md", "not valid UTF-8"},
		"too large":      {[]string{filepath.Join(tmp, "big")}, "stdin.md", "the limit is 5.0 MB"},
		"bad stdin name": {[]string{"-"}, "notes.txt", "must end in"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Collect(tc.args, strings.NewReader("x"), tc.stdinName)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestLocalImageRefs(t *testing.T) {
	files := []File{{Content: `
![local](img/a.png) ![abs](/b.png "title") ![angle](<c d.png>)
![remote](https://example.com/a.png) ![proto](//cdn/a.png) ![inline](data:image/png;base64,AAAA)
[not an image](doc.md)`}}
	if got := LocalImageRefs(files); got != 3 {
		t.Fatalf("LocalImageRefs = %d, want 3", got)
	}
}

func TestParseTTL(t *testing.T) {
	ok := map[string]time.Duration{
		"1h":   time.Hour,
		"24h":  24 * time.Hour,
		"7d":   7 * 24 * time.Hour,
		"30D":  30 * 24 * time.Hour,
		"0.5d": 12 * time.Hour,
		"90m":  90 * time.Minute,
		" 2h ": 2 * time.Hour,
	}
	for in, want := range ok {
		got, err := ParseTTL(in)
		if err != nil || got != want {
			t.Errorf("ParseTTL(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "d", "abc", "24", "-1h", "0d", "1w"} {
		if got, err := ParseTTL(in); err == nil {
			t.Errorf("ParseTTL(%q) = %v, want error", in, got)
		}
	}
}

func TestFormatTTL(t *testing.T) {
	cases := map[time.Duration]string{
		7 * 24 * time.Hour: "7d",
		36 * time.Hour:     "36h",
		90 * time.Minute:   "90m",
		90 * time.Second:   "1m30s",
	}
	for in, want := range cases {
		if got := FormatTTL(in); got != want {
			t.Errorf("FormatTTL(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestClientCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/shares" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		var body struct {
			Files []File `json:"files"`
			TTL   int64  `json:"ttl"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.TTL != 3600 || len(body.Files) != 1 || body.Files[0].Path != "a.md" {
			t.Errorf("body = %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"abc","url":"https://md.erk.im/s/abc","expiresAt":"2026-10-02T12:00:00.000Z","ttl":3600,"fileCount":1}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "secret"}
	share, err := c.Create(context.Background(), []File{{Path: "a.md", Content: "# a"}}, Expiry{TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if share.URL != "https://md.erk.im/s/abc" || share.TTL == nil || *share.TTL != 3600 || share.FileCount != 1 {
		t.Fatalf("share = %+v", share)
	}
	if want := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC); share.ExpiresAt == nil || !share.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v", share.ExpiresAt)
	}
}

func TestClientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"Invalid token","code":"unauthorized"}`))
		case http.MethodPost:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Unsupported extension","code":"invalid_path","path":"a.txt"}`))
		case http.MethodDelete:
			if r.URL.Path != "/api/shares/abc" {
				t.Errorf("path = %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "secret"}
	ctx := context.Background()

	var apiErr *APIError
	err := c.Verify(ctx)
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized || apiErr.Code != "unauthorized" {
		t.Fatalf("Verify err = %v", err)
	}
	if !strings.Contains(err.Error(), "md setup") {
		t.Fatalf("401 message lacks the setup hint: %q", err.Error())
	}

	_, err = c.Create(ctx, []File{{Path: "a.txt"}}, Expiry{TTL: time.Hour})
	if !errors.As(err, &apiErr) || apiErr.Code != "invalid_path" || apiErr.Path != "a.txt" {
		t.Fatalf("Create err = %v", err)
	}
	if err.Error() != "Unsupported extension" {
		t.Fatalf("Create message = %q", err.Error())
	}

	if err := c.Delete(ctx, "abc"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}
}

func TestClientNonJSONError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	defer srv.Close()

	err := (&Client{BaseURL: srv.URL, Token: "secret"}).Verify(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientUpdate(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path == "/api/shares/gone" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Share not found","code":"not_found"}`))
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"id":"abc","url":"https://md.erk.im/s/abc","expiresAt":"2026-10-02T12:00:00.000Z","ttl":1200,"fileCount":1}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "secret"}
	ctx := context.Background()
	files := []File{{Path: "a.md", Content: "# a"}}

	share, err := c.Update(ctx, "abc", files, Expiry{})
	if err != nil || share.ID != "abc" || share.TTL == nil || *share.TTL != 1200 {
		t.Fatalf("Update = %+v, %v", share, err)
	}
	if _, err := c.Update(ctx, "abc", files, Expiry{TTL: time.Hour}); err != nil {
		t.Fatal(err)
	}
	// A zero Expiry must omit ttl so the server keeps the original expiry.
	if _, ok := bodies[0]["ttl"]; ok {
		t.Errorf("ttl sent for a zero duration: %v", bodies[0])
	}
	if bodies[1]["ttl"] != float64(3600) {
		t.Errorf("ttl = %v, want 3600", bodies[1]["ttl"])
	}

	var apiErr *APIError
	_, err = c.Update(ctx, "gone", files, Expiry{})
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound || apiErr.Code != "not_found" {
		t.Fatalf("Update(gone) err = %v", err)
	}
}

func ptr(t time.Time) *time.Time { return &t }

func TestState(t *testing.T) {
	st := State{Path: filepath.Join(t.TempDir(), "shares.json")}
	future := time.Now().Add(time.Hour)

	if _, ok := st.Lookup("k"); ok {
		t.Fatal("Lookup on a missing file returned a record")
	}
	if err := st.Remember("k", &Share{ID: "abc", URL: "u", ExpiresAt: ptr(future)}); err != nil {
		t.Fatal(err)
	}
	if err := st.Remember("old", &Share{ID: "old", ExpiresAt: ptr(time.Now().Add(-time.Minute))}); err != nil {
		t.Fatal(err)
	}
	if err := st.Remember("kept", &Share{ID: "kept"}); err != nil {
		t.Fatal(err)
	}
	if rec, ok := st.Lookup("k"); !ok || rec.ID != "abc" {
		t.Fatalf("Lookup = %+v, %v", rec, ok)
	}
	if _, ok := st.Lookup("old"); ok {
		t.Fatal("expired record returned")
	}
	if rec, ok := st.Lookup("kept"); !ok || rec.ExpiresAt != nil {
		t.Fatalf("never-expiring record = %+v, %v", rec, ok)
	}

	// Same key replaces the record.
	if err := st.Remember("k", &Share{ID: "def", ExpiresAt: ptr(future)}); err != nil {
		t.Fatal(err)
	}
	if rec, _ := st.Lookup("k"); rec.ID != "def" {
		t.Fatalf("record not replaced: %+v", rec)
	}

	if err := st.SetExpiry("def", nil); err != nil {
		t.Fatal(err)
	}
	if rec, _ := st.Lookup("k"); rec.ExpiresAt != nil {
		t.Fatalf("SetExpiry(nil) not applied: %+v", rec)
	}

	if err := st.Forget("def"); err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Lookup("k"); ok {
		t.Fatal("record still present after Forget")
	}

	if err := os.WriteFile(st.Path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.Remember("k", &Share{ID: "abc", ExpiresAt: ptr(future)}); err != nil {
		t.Fatalf("Remember on a corrupt file: %v", err)
	}
	if rec, ok := st.Lookup("k"); !ok || rec.ID != "abc" {
		t.Fatalf("Lookup after corrupt file = %+v, %v", rec, ok)
	}
}

func TestStateFallback(t *testing.T) {
	dir := t.TempDir()
	orion := filepath.Join(dir, "md-shares.json")
	data := `{"k":{"id":"fromorion","url":"u","expiresAt":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}}`
	if err := os.WriteFile(orion, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	st := State{Path: filepath.Join(dir, "shares.json"), Fallback: orion}
	if rec, ok := st.Lookup("k"); !ok || rec.ID != "fromorion" {
		t.Fatalf("fallback Lookup = %+v, %v", rec, ok)
	}
	if err := st.Remember("k", &Share{ID: "mine"}); err != nil {
		t.Fatal(err)
	}
	if rec, _ := st.Lookup("k"); rec.ID != "mine" {
		t.Fatalf("own state should win over the fallback: %+v", rec)
	}
}

func TestStateKey(t *testing.T) {
	t.Chdir(t.TempDir())
	wd, _ := os.Getwd()

	a, err := StateKey([]string{"docs/", "b.md"}, "stdin.md")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := StateKey([]string{filepath.Join(wd, "b.md"), "./docs"}, "stdin.md")
	if a != b {
		t.Fatalf("keys differ:\n%q\n%q", a, b)
	}

	s1, _ := StateKey([]string{"-"}, "a.md")
	s2, _ := StateKey([]string{"-"}, "b.md")
	if s1 == s2 {
		t.Fatal("stdin keys should depend on the name")
	}
}

func TestParseExpiry(t *testing.T) {
	for _, in := range []string{"never", "Keep", " forever "} {
		if e, err := ParseExpiry(in); err != nil || !e.Keep {
			t.Errorf("ParseExpiry(%q) = %+v, %v", in, e, err)
		}
	}
	if e, err := ParseExpiry("7d"); err != nil || e.Keep || e.TTL != 7*24*time.Hour {
		t.Errorf("ParseExpiry(7d) = %+v, %v", e, err)
	}
	if _, err := ParseExpiry("soon"); err == nil {
		t.Error("ParseExpiry(soon) should fail")
	}
}

func TestClientKeepAndPatch(t *testing.T) {
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = append(got, body)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/shares":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"abc","url":"u","expiresAt":null,"ttl":null,"fileCount":1}`))
		case "PATCH /api/shares/abc":
			_, _ = w.Write([]byte(`{"id":"abc","url":"u","title":"T","expiresAt":"2026-10-02T12:00:00Z","ttl":60,"fileCount":1}`))
		case "GET /api/me/shares":
			_, _ = w.Write([]byte(`{"shares":[{"id":"abc","url":"u","title":"T","fileCount":1,"createdAt":"2026-10-01T12:00:00Z","expiresAt":null}]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "secret"}
	ctx := context.Background()

	share, err := c.Create(ctx, []File{{Path: "a.md", Content: "x"}}, Expiry{Keep: true})
	if err != nil || share.ExpiresAt != nil || share.TTL != nil {
		t.Fatalf("Create keep = %+v, %v", share, err)
	}
	if got[0]["keep"] != true || got[0]["ttl"] != nil {
		t.Fatalf("keep body = %v", got[0])
	}

	title, empty := "T", ""
	for i, p := range []Patch{{Extend: time.Hour}, {TTL: 2 * time.Hour}, {Keep: true}, {Title: &title}, {Title: &empty}} {
		if _, err := c.Patch(ctx, "abc", p); err != nil {
			t.Fatalf("Patch %d: %v", i, err)
		}
	}
	want := []string{`{"extend":3600}`, `{"ttl":7200}`, `{"keep":true}`, `{"title":"T"}`, `{"title":null}`}
	for i, w := range want {
		b, _ := json.Marshal(got[i+1])
		if string(b) != w {
			t.Errorf("patch body %d = %s, want %s", i, b, w)
		}
	}

	shares, err := c.List(ctx)
	if err != nil || len(shares) != 1 || shares[0].ExpiresAt != nil || shares[0].Title != "T" {
		t.Fatalf("List = %+v, %v", shares, err)
	}
}

func TestShareID(t *testing.T) {
	cases := map[string]string{
		"https://md.erk.im/s/AbC_-123?present": "AbC_-123",
		"https://md.erk.im/s/abc/":             "abc",
		"abc":                                  "abc",
		" abc ":                                "abc",
		"":                                     "",
	}
	for in, want := range cases {
		if got := ShareID(in); got != want {
			t.Errorf("ShareID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAPIErrorHints(t *testing.T) {
	if msg := (&APIError{Status: 400, Message: "no", Code: "keep_requires_owner"}).Error(); !strings.Contains(msg, "personal token") {
		t.Errorf("keep_requires_owner hint missing: %q", msg)
	}
	if !IsNotFound(&APIError{Status: 404}) || IsNotFound(errors.New("x")) {
		t.Error("IsNotFound misclassifies")
	}
}

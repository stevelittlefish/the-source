package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAPI records every request and answers with the handler's response.
func fakeAPI(t *testing.T, h http.HandlerFunc) (*[]*http.Request, func() (string, error)) {
	t.Helper()
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &seen, func() (string, error) { return srv.URL + "/", nil }
}

func jsonOK(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}
}

func execute(args []string, base func() (string, error)) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, base)
	return code, out.String(), errOut.String()
}

func TestSearchBuildsQueryWithFlagsAfterWords(t *testing.T) {
	seen, base := fakeAPI(t, jsonOK(`{"books":[],"total":0,"next_cursor":""}`))
	code, out, stderr := execute([]string{"books", "search", "jane", "austen", "--limit", "5", "--language", "all"}, base)
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	r := (*seen)[0]
	if r.URL.Path != "/api/v1/books" {
		t.Fatalf("path %q", r.URL.Path)
	}
	q := r.URL.Query()
	if q.Get("q") != "jane austen" || q.Get("limit") != "5" || q.Get("language") != "all" || len(q) != 3 {
		t.Fatalf("query %v", q)
	}
	if !strings.Contains(out, "\n  \"books\": []") {
		t.Fatalf("output not indented JSON: %q", out)
	}
}

func TestSearchWithoutWordsOmitsQ(t *testing.T) {
	seen, base := fakeAPI(t, jsonOK(`{"songs":[]}`))
	if code, _, stderr := execute([]string{"lyrics", "search", "--tag", "rap", "--views-from", "1000"}, base); code != exitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	q := (*seen)[0].URL.Query()
	if q.Has("q") || q.Get("tag") != "rap" || q.Get("views_from") != "1000" {
		t.Fatalf("query %v", q)
	}
}

func TestBookTextStreamsWithRange(t *testing.T) {
	seen, base := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("It is a truth"))
	})
	code, out, _ := execute([]string{"books", "text", "1342", "--offset", "100", "--bytes", "50"}, base)
	if code != exitOK || out != "It is a truth" {
		t.Fatalf("exit %d, out %q", code, out)
	}
	r := (*seen)[0]
	if r.URL.Path != "/api/v1/books/1342/text" || r.Header.Get("Range") != "bytes=100-149" {
		t.Fatalf("path %q range %q", r.URL.Path, r.Header.Get("Range"))
	}
}

func TestExcerptRetriesUnsuitableThenSucceeds(t *testing.T) {
	calls := 0
	seen, base := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Write([]byte(`{"error":{"code":"no_suitable_excerpt","message":"nope"}}`))
			return
		}
		jsonOK(`{"book":{"id":1},"paragraphs":["p"]}`)(w, r)
	})
	code, out, _ := execute([]string{"books", "excerpt", "--paragraphs", "2"}, base)
	if code != exitOK || len(*seen) != 3 || !strings.Contains(out, `"paragraphs"`) {
		t.Fatalf("exit %d after %d calls, out %q", code, len(*seen), out)
	}
	if (*seen)[0].URL.Query().Get("paragraphs") != "2" {
		t.Fatalf("query %v", (*seen)[0].URL.Query())
	}
}

func TestExcerptRetriesAreBounded(t *testing.T) {
	seen, base := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":{"code":"no_suitable_excerpt","message":"nope"}}`))
	})
	code, _, stderr := execute([]string{"lyrics", "excerpt", "--retries", "1"}, base)
	if code != exitAPIError || len(*seen) != 2 || !strings.Contains(stderr, "no_suitable_excerpt") {
		t.Fatalf("exit %d after %d calls, stderr %q", code, len(*seen), stderr)
	}
}

func TestAPIErrorGoesToStderrAsJSON(t *testing.T) {
	_, base := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":"book_not_found","message":"No book."}}`))
	})
	code, out, stderr := execute([]string{"books", "get", "999999"}, base)
	if code != exitAPIError || out != "" || !strings.Contains(stderr, `"code": "book_not_found"`) {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
}

func TestNonJSONErrorIsWrapped(t *testing.T) {
	_, base := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
	})
	code, _, stderr := execute([]string{"books", "text", "1", "--offset", "99999999"}, base)
	var e struct {
		Error struct{ Code, Message string }
	}
	if code != exitAPIError || json.Unmarshal([]byte(stderr), &e) != nil || e.Error.Code != "http_416" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestUsageErrors(t *testing.T) {
	_, base := fakeAPI(t, jsonOK(`{}`))
	for _, args := range [][]string{
		{"books", "get"},
		{"books", "get", "abc"},
		{"books", "get", "0"},
		{"books", "random", "extra"},
		{"books", "search", "--nope", "1"},
		{"lyrics", "text", "1", "--bytes", "5"}, // lyrics text has no ranges
		{"frobnicate"},
		{"raw", "api/v1/books"},
	} {
		if code, _, _ := execute(args, base); code != exitUsage {
			t.Errorf("%q: exit %d, want %d", args, code, exitUsage)
		}
	}
}

func TestRawPassesPathThrough(t *testing.T) {
	seen, base := fakeAPI(t, jsonOK(`{"status":"ok"}`))
	if code, _, _ := execute([]string{"raw", "/api/v1/lyrics/random?views_from=5"}, base); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if got := (*seen)[0].URL.String(); got != "/api/v1/lyrics/random?views_from=5" {
		t.Fatalf("url %q", got)
	}
}

func TestHelpMentionsEveryCommand(t *testing.T) {
	code, out, _ := execute([]string{"help"}, nil)
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	for _, c := range commands {
		if !strings.Contains(out, "thesource "+c.name) {
			t.Errorf("help missing %q", c.name)
		}
	}
	if code, _, stderr := execute([]string{"books", "search", "--help"}, nil); code != exitOK || !strings.Contains(stderr, "--available") {
		t.Fatalf("command help: exit %d, %q", code, stderr)
	}
}

func TestUnreachableServerIsSetupError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	code, _, stderr := execute([]string{"health"}, func() (string, error) { return url, nil })
	if code != exitSetup || !strings.Contains(stderr, "cannot reach") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestShippedConfigParses(t *testing.T) {
	// The committed thesource.toml is the one that gets copied beside the binary.
	got, err := readBaseURL(configName)
	if err != nil || got != "http://lemon.com:45068/" {
		t.Fatalf("base_url %q, err %v", got, err)
	}
}

func TestBadConfigExplainsItself(t *testing.T) {
	path := filepath.Join(t.TempDir(), configName)
	if _, err := readBaseURL(path); err == nil || !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("missing config err %v", err)
	}
	os.WriteFile(path, []byte("base_url = \"lemon:45068\"\n"), 0o600)
	if _, err := readBaseURL(path); err == nil {
		t.Fatal("accepted a base_url without a scheme")
	}
}

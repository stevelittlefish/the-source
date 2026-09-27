package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stevelittlefish/the-source/internal/markov"
)

func markovServer(t *testing.T) *Server {
	t.Helper()
	s := testServer(t)
	chains, err := markov.LoadDir("../../testdata/markov")
	if err != nil {
		t.Fatal(err)
	}
	s.SetChains(chains)
	return s
}

type markovBatch struct {
	Chain   string          `json:"chain"`
	Kind    string          `json:"kind"`
	Seed    *uint64         `json:"seed"`
	Results []markov.Result `json:"results"`
}

func getMarkov(t *testing.T, s *Server, path string) (int, markovBatch, string) {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	var out markovBatch
	if w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, out, w.Body.String()
}

func TestMarkovListsChains(t *testing.T) {
	w := httptest.NewRecorder()
	markovServer(t).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/markov", nil))
	var out struct {
		Chains []struct {
			Name, Kind string
			Trained    int `json:"trained_on"`
		}
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || len(out.Chains) != 2 {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if out.Chains[0].Name != "pop-titles" || out.Chains[0].Kind != "title" || out.Chains[1].Name != "pop-verses" || out.Chains[1].Kind != "verse" {
		t.Fatalf("unexpected chains: %+v", out.Chains)
	}
}

func TestMarkovListWithoutChainsIsEmpty(t *testing.T) {
	w := httptest.NewRecorder()
	testServer(t).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/markov", nil))
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"chains":[]}` {
		t.Fatalf("empty list: %d %s", w.Code, w.Body)
	}
}

func TestMarkovBatchAndSeed(t *testing.T) {
	s := markovServer(t)
	code, one, body := getMarkov(t, s, "/api/v1/markov/pop-titles")
	if code != 200 || len(one.Results) != 1 || one.Kind != "title" || one.Seed != nil {
		t.Fatalf("default batch: %d %s", code, body)
	}
	code, a, body := getMarkov(t, s, "/api/v1/markov/pop-verses?count=25&seed=42")
	if code != 200 || len(a.Results) != 25 || a.Kind != "verse" || a.Seed == nil || *a.Seed != 42 {
		t.Fatalf("verse batch: %d %s", code, body)
	}
	for _, r := range a.Results {
		if len(r.Lines) < 4 || len(r.Lines) > 8 || r.Text != strings.Join(r.Lines, "\n") {
			t.Fatalf("bad verse: %+v", r)
		}
	}
	_, b, _ := getMarkov(t, s, "/api/v1/markov/pop-verses?count=25&seed=42")
	for i := range a.Results {
		if a.Results[i].Text != b.Results[i].Text {
			t.Fatal("same seed gave a different batch")
		}
	}
}

func TestMarkovRejectsBadRequests(t *testing.T) {
	s := markovServer(t)
	for path, want := range map[string]int{
		"/api/v1/markov/nope":                       404,
		"/api/v1/markov/pop-titles?count=0":         400,
		"/api/v1/markov/pop-titles?count=101":       400,
		"/api/v1/markov/pop-titles?count=x":         400,
		"/api/v1/markov/pop-titles?seed=-1":         400,
		"/api/v1/markov/pop-titles?colour=blue":     400,
		"/api/v1/markov/pop-titles?count=1&count=2": 400,
		"/api/v1/markov?count=2":                    400,
		"/api/v1/markov/pop-titles/extra":           404,
	} {
		if code, _, body := getMarkov(t, s, path); code != want {
			t.Errorf("%s: got %d, want %d (%s)", path, code, want, body)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/markov/pop-titles", nil))
	if w.Code != 405 {
		t.Errorf("POST: %d", w.Code)
	}
}

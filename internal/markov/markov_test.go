package markov

import (
	"bytes"
	"compress/gzip"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtures(t *testing.T) map[string]*Chain {
	t.Helper()
	chains, err := LoadDir("../../testdata/markov")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*Chain{}
	for _, c := range chains {
		byName[c.Name] = c
	}
	return byName
}

func TestLoadDirFindsBothKinds(t *testing.T) {
	chains := fixtures(t)
	for name, kind := range map[string]string{"pop-titles": KindTitle, "pop-verses": KindVerse} {
		c := chains[name]
		if c == nil || c.Kind != kind || c.Trained < 900 {
			t.Fatalf("%s: missing, wrong kind or too small", name)
		}
	}
}

func TestTitlesAreNewAndCapitalised(t *testing.T) {
	c := fixtures(t)["pop-titles"]
	rng := rand.New(rand.NewPCG(1, 2))
	for range 200 {
		r, err := c.Generate(rng)
		if err != nil {
			t.Fatal(err)
		}
		words := strings.Fields(r.Text)
		if len(words) < 2 || len(words) > titleMaxWords || len(r.Lines) != 1 || r.Lines[0] != r.Text {
			t.Fatalf("bad title shape: %+v", r)
		}
		if _, copied := c.titles[strings.ToLower(r.Text)]; copied {
			t.Fatalf("copied a training title: %q", r.Text)
		}
		for _, w := range words {
			if first := []rune(w)[0]; strings.ToUpper(string(first)) != string(first) {
				t.Fatalf("word not capitalised in %q", r.Text)
			}
		}
	}
}

func TestVersesHaveFourToEightOriginalLines(t *testing.T) {
	c := fixtures(t)["pop-verses"]
	rng := rand.New(rand.NewPCG(3, 4))
	for range 100 {
		r, err := c.Generate(rng)
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Lines) < verseMinLines || len(r.Lines) > verseMaxLines || r.Text != strings.Join(r.Lines, "\n") {
			t.Fatalf("bad verse shape: %+v", r)
		}
		for _, line := range r.Lines {
			words := strings.Fields(strings.ToLower(line))
			if _, copied := c.copies[lineHash(words)]; len(words) >= copyMinWords && copied {
				t.Fatalf("copied a real line: %q", line)
			}
		}
	}
}

func TestSameSeedSameResults(t *testing.T) {
	c := fixtures(t)["pop-verses"]
	a, _ := c.Generate(rand.New(rand.NewPCG(7, 7)))
	b, _ := c.Generate(rand.New(rand.NewPCG(7, 7)))
	if a.Text != b.Text {
		t.Fatalf("seeded runs differ:\n%s\n---\n%s", a.Text, b.Text)
	}
}

func TestLineHashMatchesTrainer(t *testing.T) {
	// Python: hashlib.sha1("i walk the line".encode()).hexdigest()[:12]
	want, err := hashPrefix("dd6f4649a091")
	if err != nil {
		t.Fatal(err)
	}
	if got := lineHash([]string{"i", "walk", "the", "line"}); got != want {
		t.Fatalf("lineHash = %x, want %x", got, want)
	}
}

func TestDressLine(t *testing.T) {
	if got := dressLine([]string{"i'm", "sure", "i", "saw", "ice"}); got != "I'm sure I saw ice" {
		t.Fatalf("dressLine: %q", got)
	}
	if got := capitalise("élan"); got != "Élan" {
		t.Fatalf("capitalise: %q", got)
	}
}

func TestRejectsModelsItDoesNotUnderstand(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"neither.json.gz": `{"version":2,"transitions":{"\u0002\u001f\u0002":{"a":1}}}`,
		"future.json.gz":  `{"version":9,"titles":1,"source_titles":[],"transitions":{"\u0002\u001f\u0002":{"a":1}}}`,
		"empty.json.gz":   `{"version":2,"titles":1,"source_titles":[],"transitions":{}}`,
	} {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		zw.Write([]byte(body))
		zw.Close()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Errorf("%s loaded; it should not have", name)
		}
	}
}

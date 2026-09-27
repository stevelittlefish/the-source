// Package markov serves word-pair Markov chains trained by the
// SongInspirationEngine: song titles and song verses, one gzipped JSON model
// per chain. Generation follows the Python originals rule for rule, including
// their refusal to hand back a real song's words: a title chain rejects exact
// training titles, a verse chain rejects any line of four or more words that
// appears in a real verse.
//
// Models are streamed while loading and packed into integer tables, because
// the verse models hold about three million transitions and a map of maps of
// strings would cost a gigabyte to say "baby" a lot.
package markov

import (
	"bufio"
	"compress/gzip"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Tokens with special meaning, exactly as the trainers write them.
const (
	startToken   = "\x02"
	endToken     = "\x03"
	newlineToken = "\x1e"
	separator    = "\x1f"
)

// The interned IDs of the special tokens; intern() hands them out first.
const (
	startID uint32 = iota
	endID
	newlineID
)

const (
	KindTitle = "title"
	KindVerse = "verse"

	titleMaxWords    = 12
	titleAttempts    = 1000
	verseMinLines    = 4
	verseMaxLines    = 8
	verseMaxWords    = 20
	verseAttempts    = 2000
	copyMinWords     = 4
	titleModelFormat = 2
	verseModelFormat = 1
)

// ErrNoResult means the chain could not produce an original result within its
// attempt budget. Rare with real models; common with a model of three songs.
var ErrNoResult = errors.New("markov: no original result found")

type edge struct {
	token uint32
	cum   uint32 // running total of counts, for weighted choice by binary search
}

// Chain is one loaded model. Generation only reads it, so one Chain serves any
// number of concurrent requests.
type Chain struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Trained int    `json:"trained_on"` // titles or verses in the training set

	words  []string
	next   map[uint64][]edge
	titles map[string]struct{} // title chains: every training title, lower case
	copies map[uint64]struct{} // verse chains: first 48 bits of each guarded line's SHA-1
}

// Result is one generated title or verse. A title is a single line.
type Result struct {
	Text  string   `json:"text"`
	Lines []string `json:"lines"`
}

func key(previous, current uint32) uint64 { return uint64(previous)<<32 | uint64(current) }

// LoadDir loads every *.json.gz in dir as a chain named after its file. If
// logf is not nil it narrates each file, because a 20 MB model takes long
// enough to look like a hang.
func LoadDir(dir string, logf func(string, ...any)) ([]*Chain, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json.gz"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if logf == nil {
		logf = func(string, ...any) {}
	}
	logf("markov: %d model files in %s", len(paths), dir)
	chains := make([]*Chain, 0, len(paths))
	for _, path := range paths {
		started := time.Now()
		size := int64(0)
		if info, err := os.Stat(path); err == nil {
			size = info.Size()
		}
		logf("markov: loading %s (%.1f MB)", filepath.Base(path), float64(size)/1e6)
		chain, err := Load(path)
		if err != nil {
			return nil, err
		}
		edges := 0
		for _, e := range chain.next {
			edges += len(e)
		}
		logf("markov: %s ready in %s: %s chain, trained on %d, %d words, %d states, %d transitions",
			chain.Name, time.Since(started).Round(time.Millisecond), chain.Kind, chain.Trained, len(chain.words), len(chain.next), edges)
		chains = append(chains, chain)
	}
	return chains, nil
}

// Load reads one model. The kind comes from the model's contents: title models
// carry source_titles, verse models carry copy_lines.
func Load(path string) (*Chain, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zipped, err := gzip.NewReader(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer zipped.Close()
	c := &Chain{Name: strings.TrimSuffix(filepath.Base(path), ".json.gz"), next: map[uint64][]edge{}}
	ids := map[string]uint32{}
	intern := func(token string) uint32 {
		if id, ok := ids[token]; ok {
			return id
		}
		id := uint32(len(c.words))
		ids[token] = id
		c.words = append(c.words, token)
		return id
	}
	intern(startToken)
	intern(endToken)
	intern(newlineToken)
	if err := c.decode(json.NewDecoder(bufio.NewReaderSize(zipped, 1<<20)), intern); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	switch {
	case c.titles != nil && c.copies == nil:
		c.Kind = KindTitle
	case c.copies != nil && c.titles == nil:
		c.Kind = KindVerse
	default:
		return nil, fmt.Errorf("%s: not a title or verse model", path)
	}
	if len(c.next[key(startID, startID)]) == 0 {
		return nil, fmt.Errorf("%s: model has no starting transitions", path)
	}
	return c, nil
}

// decode walks the top-level object token by token, so the transitions never
// exist as a decoded map of maps.
func (c *Chain) decode(dec *json.Decoder, intern func(string) uint32) error {
	if err := expectDelim(dec, '{'); err != nil {
		return err
	}
	version := -1
	for dec.More() {
		name, err := stringToken(dec)
		if err != nil {
			return err
		}
		switch name {
		case "version":
			if err := dec.Decode(&version); err != nil {
				return err
			}
		case "titles", "verses":
			if err := dec.Decode(&c.Trained); err != nil {
				return err
			}
		case "source_titles":
			var titles []string
			if err := dec.Decode(&titles); err != nil {
				return err
			}
			c.titles = make(map[string]struct{}, len(titles))
			for _, title := range titles {
				c.titles[title] = struct{}{}
			}
		case "copy_lines":
			var hashes []string
			if err := dec.Decode(&hashes); err != nil {
				return err
			}
			c.copies = make(map[uint64]struct{}, len(hashes))
			for _, h := range hashes {
				n, err := hashPrefix(h)
				if err != nil {
					return err
				}
				c.copies[n] = struct{}{}
			}
		case "transitions":
			if err := c.decodeTransitions(dec, intern); err != nil {
				return err
			}
		default:
			var skip json.RawMessage // min_views, max_junk and whatever the trainers add next
			if err := dec.Decode(&skip); err != nil {
				return err
			}
		}
	}
	if (c.titles != nil && version != titleModelFormat) || (c.copies != nil && version != verseModelFormat) {
		return fmt.Errorf("unsupported model version %d", version)
	}
	return nil
}

func (c *Chain) decodeTransitions(dec *json.Decoder, intern func(string) uint32) error {
	if err := expectDelim(dec, '{'); err != nil {
		return err
	}
	for dec.More() {
		state, err := stringToken(dec)
		if err != nil {
			return err
		}
		previous, current, ok := strings.Cut(state, separator)
		if !ok {
			return fmt.Errorf("malformed state %q", state)
		}
		if err := expectDelim(dec, '{'); err != nil {
			return err
		}
		var edges []edge
		var total uint32
		for dec.More() {
			token, err := stringToken(dec)
			if err != nil {
				return err
			}
			var count uint32
			if err := dec.Decode(&count); err != nil {
				return err
			}
			if count == 0 {
				return fmt.Errorf("zero count in state %q", state)
			}
			total += count
			edges = append(edges, edge{intern(token), total})
		}
		if err := expectDelim(dec, '}'); err != nil {
			return err
		}
		if len(edges) > 0 {
			c.next[key(intern(previous), intern(current))] = slices.Clip(edges)
		}
	}
	return expectDelim(dec, '}')
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("expected %q, found %v", want, token)
	}
	return nil
}

func stringToken(dec *json.Decoder) (string, error) {
	token, err := dec.Token()
	if err != nil {
		return "", err
	}
	s, ok := token.(string)
	if !ok {
		return "", fmt.Errorf("expected a string, found %v", token)
	}
	return s, nil
}

// hashPrefix turns the trainer's 12 hex characters into a comparable integer.
func hashPrefix(h string) (uint64, error) {
	raw, err := hex.DecodeString(h)
	if err != nil || len(raw) != 6 {
		return 0, fmt.Errorf("malformed copy_lines hash %q", h)
	}
	return uint64(binary.BigEndian.Uint16(raw[:2]))<<32 | uint64(binary.BigEndian.Uint32(raw[2:])), nil
}

func lineHash(words []string) uint64 {
	sum := sha1.Sum([]byte(strings.Join(words, " ")))
	return uint64(binary.BigEndian.Uint16(sum[:2]))<<32 | uint64(binary.BigEndian.Uint32(sum[2:6]))
}

// choose picks a next token with probability proportional to its count, as
// the Python generator's walk down the counts does.
func (c *Chain) choose(edges []edge, rng *rand.Rand) uint32 {
	pick := uint32(rng.IntN(int(edges[len(edges)-1].cum)))
	return edges[sort.Search(len(edges), func(i int) bool { return edges[i].cum > pick })].token
}

// Generate returns one result, or ErrNoResult.
func (c *Chain) Generate(rng *rand.Rand) (Result, error) {
	if c.Kind == KindTitle {
		return c.title(rng)
	}
	return c.verse(rng)
}

func (c *Chain) title(rng *rand.Rand) (Result, error) {
	for range titleAttempts {
		previous, current := startID, startID
		var words []string
		for len(words) <= titleMaxWords {
			edges := c.next[key(previous, current)]
			if len(edges) == 0 {
				break
			}
			token := c.choose(edges, rng)
			if token == endID {
				if _, copied := c.titles[strings.Join(words, " ")]; len(words) >= 2 && !copied {
					for i, word := range words {
						words[i] = capitalise(word)
					}
					text := strings.Join(words, " ")
					return Result{Text: text, Lines: []string{text}}, nil
				}
				break
			}
			if len(words) == titleMaxWords {
				break
			}
			words = append(words, c.words[token])
			previous, current = current, token
		}
	}
	return Result{}, ErrNoResult
}

func (c *Chain) verse(rng *rand.Rand) (Result, error) {
	for range verseAttempts {
		previous, current := startID, startID
		var lines [][]string
		var words []string
		token := startID
		for {
			edges := c.next[key(previous, current)]
			if len(edges) == 0 {
				break
			}
			token = c.choose(edges, rng)
			if token == newlineID || token == endID {
				if len(words) == 0 {
					break
				}
				if _, copied := c.copies[lineHash(words)]; len(words) >= copyMinWords && copied {
					break // A real song's line. Flattering, but no.
				}
				lines = append(lines, words)
				words = nil
				if token == endID || len(lines) > verseMaxLines {
					break
				}
			} else {
				words = append(words, c.words[token])
				if len(words) > verseMaxWords {
					break
				}
			}
			previous, current = current, token
		}
		if token == endID && len(words) == 0 && len(lines) >= verseMinLines && len(lines) <= verseMaxLines {
			out := make([]string, len(lines))
			for i, line := range lines {
				out[i] = dressLine(line)
			}
			return Result{Text: strings.Join(out, "\n"), Lines: out}, nil
		}
	}
	return Result{}, ErrNoResult
}

// dressLine capitalises I, I'm, I'll... and the first letter of the line.
func dressLine(words []string) string {
	out := make([]string, len(words))
	for i, word := range words {
		if word == "i" || strings.HasPrefix(word, "i'") {
			word = capitalise(word)
		}
		out[i] = word
	}
	return capitalise(strings.Join(out, " "))
}

func capitalise(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

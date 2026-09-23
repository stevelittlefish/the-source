// Command thesource is a small command-line client for The Source API, built so
// that scripts and LLM agents can ask for books and lyrics without composing
// URLs by hand. It reads the server address from thesource.toml in the same
// directory as the binary:
//
//	base_url = "http://lemon.com:45068/"
//
// Output is the API's own JSON (indented) on stdout, or plain text for book and
// song bodies. API errors print their JSON to stderr and exit 1; usage mistakes
// exit 2; configuration and network failures exit 3. `thesource help` prints the
// whole reference in one go, so a single call teaches a new reader everything.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const configName = "thesource.toml"

const (
	exitOK       = 0
	exitAPIError = 1
	exitUsage    = 2
	exitSetup    = 3
)

// option is an API query parameter exposed as a --flag. Values pass straight
// through; the server owns validation and says so in invalid_query errors.
type option struct {
	param string
	usage string
}

func (o option) flag() string { return strings.ReplaceAll(o.param, "_", "-") }

var options = map[string]option{
	"language":   {"language", "language code, or all. Books default to en; lyrics default to all"},
	"available":  {"available", "true = text installed, false = metadata only; omit for both"},
	"limit":      {"limit", "results per page, 1-100 (default 25)"},
	"cursor":     {"cursor", "next_cursor from the previous page (same filters and limit)"},
	"year_from":  {"year_from", "original publication year >= this (unknown years are excluded)"},
	"year_to":    {"year_to", "original publication year <= this"},
	"paragraphs": {"paragraphs", "paragraphs in the excerpt, 1-10 (default 3)"},
	"tag":        {"tag", "genre: rap, pop, rock, rb, country, ... (see: lyrics tags)"},
	"views_from": {"views_from", "minimum Genius view count"},
	"views_to":   {"views_to", "maximum Genius view count"},
	"field":      {"field", "restrict the search to title or artist (default: title, artist and lyrics)"},
}

type command struct {
	name    string   // "books search"
	args    string   // positional arguments, for help
	summary string   // one line
	path    string   // API path; {id} is replaced by the first positional
	params  []string // keys into options
	words   bool     // positional words become q
	text    bool     // plain-text body, streamed
	ranged  bool     // supports --offset/--bytes
	retry   bool     // retry 422 no_suitable_excerpt
}

var commands = []command{
	{name: "books search", args: "[WORDS...]", summary: "Search book metadata (title, author, subject, bookshelf; every word must match)",
		path: "/api/v1/books", params: []string{"language", "available", "limit", "cursor"}, words: true},
	{name: "books get", args: "ID", summary: "Metadata for one Gutenberg book",
		path: "/api/v1/books/{id}"},
	{name: "books text", args: "ID", summary: "Plain text of a book, Gutenberg header included (large: use --bytes)",
		path: "/api/v1/books/{id}/text", text: true, ranged: true},
	{name: "books random", summary: "Metadata for a random book with installed text",
		path: "/api/v1/books/random", params: []string{"language", "year_from", "year_to"}},
	{name: "books excerpt", summary: "Consecutive prose paragraphs from a random book",
		path: "/api/v1/books/excerpts/random", params: []string{"paragraphs", "language", "year_from", "year_to"}, retry: true},
	{name: "books languages", summary: "Every language in the book catalog",
		path: "/api/v1/books/languages"},
	{name: "lyrics search", args: "[WORDS...]", summary: "Full-text song search over title, artist and lyrics, most relevant first",
		path: "/api/v1/lyrics", params: []string{"field", "language", "tag", "views_from", "views_to", "limit", "cursor"}, words: true},
	{name: "lyrics get", args: "ID", summary: "Metadata for one song",
		path: "/api/v1/lyrics/{id}"},
	{name: "lyrics text", args: "ID", summary: "Full lyrics of a song as plain text",
		path: "/api/v1/lyrics/{id}/text", text: true},
	{name: "lyrics random", summary: "Metadata for a random song",
		path: "/api/v1/lyrics/random", params: []string{"language", "tag", "views_from", "views_to"}},
	{name: "lyrics excerpt", summary: "One stanza from a random song",
		path: "/api/v1/lyrics/excerpts/random", params: []string{"language", "tag", "views_from", "views_to"}, retry: true},
	{name: "lyrics tags", summary: "Every genre tag",
		path: "/api/v1/lyrics/tags"},
	{name: "lyrics languages", summary: "Every song language",
		path: "/api/v1/lyrics/languages"},
	{name: "health", summary: "Check the server is up",
		path: "/health"},
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, loadBaseURL))
}

// run is main without the process: tests hand it a fake server's URL.
func run(args []string, stdout, stderr io.Writer, baseURL func() (string, error)) int {
	if len(args) == 0 || (isHelp(args[0]) && len(args) == 1) {
		fmt.Fprint(stdout, helpText())
		return exitOK
	}
	if isHelp(args[0]) {
		return topicHelp(args[1:], stdout, stderr)
	}
	switch args[0] {
	case "config":
		path, _ := configPath()
		base, err := baseURL()
		if err != nil {
			fmt.Fprintln(stderr, "thesource:", err)
			return exitSetup
		}
		fmt.Fprintf(stdout, "config: %s\nbase_url: %s\n", path, base)
		return exitOK
	case "raw":
		if len(args) != 2 || !strings.HasPrefix(args[1], "/") {
			fmt.Fprintln(stderr, "usage: thesource raw /api/v1/PATH?QUERY")
			return exitUsage
		}
		return fetch(baseURL, args[1], nil, false, 0, stdout, stderr)
	}

	cmd, rest, ok := findCommand(args)
	if !ok && groups[args[0]] != "" {
		// `thesource books` or `thesource books help`: list what books can do.
		if len(args) == 1 || isHelp(args[1]) {
			fmt.Fprint(stdout, groupHelp(args[0]))
			return exitOK
		}
		fmt.Fprintf(stderr, "thesource: unknown command %q\n\n%s", strings.Join(args[:2], " "), groupHelp(args[0]))
		return exitUsage
	}
	if !ok {
		fmt.Fprintf(stderr, "thesource: unknown command %q; run `thesource help`\n", strings.Join(args[:min(2, len(args))], " "))
		return exitUsage
	}
	return runCommand(cmd, rest, baseURL, stdout, stderr)
}

// groups are the first words of two-word commands, with a line for their help.
var groups = map[string]string{
	"books":  "Project Gutenberg books: metadata search, full texts, random picks and excerpts",
	"lyrics": "Song lyrics: full-text search, lyrics, random songs and stanzas",
}

// topicHelp answers `thesource help books` and `thesource help books search`.
func topicHelp(topic []string, stdout, stderr io.Writer) int {
	if c, rest, ok := findCommand(topic); ok && len(rest) == 0 {
		fmt.Fprint(stdout, commandHelp(c))
		return exitOK
	}
	if len(topic) == 1 && groups[topic[0]] != "" {
		fmt.Fprint(stdout, groupHelp(topic[0]))
		return exitOK
	}
	fmt.Fprintf(stderr, "thesource: no help for %q; run `thesource help`\n", strings.Join(topic, " "))
	return exitUsage
}

func groupHelp(group string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "thesource %s - %s\n", group, groups[group])
	for _, c := range commands {
		if strings.HasPrefix(c.name, group+" ") {
			b.WriteString("\n" + commandHelp(c))
		}
	}
	b.WriteString("\nRun `thesource help` for output shapes, exit codes and examples.\n")
	return b.String()
}

func isHelp(s string) bool { return s == "help" || s == "-h" || s == "--help" || s == "-help" }

func findCommand(args []string) (command, []string, bool) {
	for _, c := range commands {
		n := len(strings.Fields(c.name))
		if len(args) >= n && strings.Join(args[:n], " ") == c.name {
			return c, args[n:], true
		}
	}
	return command{}, nil, false
}

func runCommand(c command, args []string, baseURL func() (string, error), stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("thesource "+c.name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, commandHelp(c)) }
	values := map[string]*string{}
	for _, key := range c.params {
		o := options[key]
		values[key] = fs.String(o.flag(), "", o.usage)
	}
	var offset, size int64
	if c.ranged {
		fs.Int64Var(&offset, "offset", 0, "start at this byte")
		fs.Int64Var(&size, "bytes", 0, "return at most this many bytes")
	}
	retries := 0
	if c.retry {
		fs.IntVar(&retries, "retries", 3, "extra attempts when no suitable excerpt is found")
	}

	positional, err := parseInterspersed(fs, args)
	if err == flag.ErrHelp {
		return exitOK
	}
	if err != nil {
		return exitUsage
	}

	path := c.path
	q := url.Values{}
	switch {
	case strings.Contains(path, "{id}"):
		if len(positional) != 1 {
			fmt.Fprintf(stderr, "usage: thesource %s %s\n", c.name, c.args)
			return exitUsage
		}
		if n, err := strconv.ParseUint(positional[0], 10, 64); err != nil || n == 0 {
			fmt.Fprintf(stderr, "thesource: ID must be a positive integer, not %q\n", positional[0])
			return exitUsage
		}
		path = strings.Replace(path, "{id}", positional[0], 1)
	case c.words:
		if words := strings.Join(positional, " "); strings.TrimSpace(words) != "" {
			q.Set("q", words)
		}
	case len(positional) > 0:
		fmt.Fprintf(stderr, "thesource: %s takes no arguments, got %q\n", c.name, positional)
		return exitUsage
	}
	for key, v := range values {
		if *v != "" {
			q.Set(key, *v)
		}
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	header := http.Header{}
	if size < 0 || offset < 0 {
		fmt.Fprintln(stderr, "thesource: --offset and --bytes must not be negative")
		return exitUsage
	}
	if size > 0 {
		header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+size-1))
	} else if offset > 0 {
		header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	return fetch(baseURL, path, header, c.text, retries, stdout, stderr)
}

// parseInterspersed lets flags follow positional words, so
// `thesource books search jane austen --limit 5` does what it looks like.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

var client = &http.Client{Timeout: 60 * time.Second}

func fetch(baseURL func() (string, error), path string, header http.Header, text bool, retries int, stdout, stderr io.Writer) int {
	base, err := baseURL()
	if err != nil {
		fmt.Fprintln(stderr, "thesource:", err)
		return exitSetup
	}
	target := strings.TrimRight(base, "/") + path

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			fmt.Fprintln(stderr, "thesource:", err)
			return exitSetup
		}
		for k, v := range header {
			req.Header[k] = v
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(stderr, "thesource: cannot reach %s: %v\n", base, err)
			return exitSetup
		}

		if resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnprocessableEntity && attempt < retries {
				continue
			}
			if !json.Valid(body) {
				// Non-JSON failures (a 416, a proxy page) get wrapped so stderr
				// is always one JSON object with a code to branch on.
				body, _ = json.Marshal(map[string]any{"error": map[string]any{
					"code": "http_" + strconv.Itoa(resp.StatusCode), "message": strings.TrimSpace(string(body))}})
			}
			writeJSON(stderr, body)
			return exitAPIError
		}

		defer resp.Body.Close()
		if text {
			if _, err := io.Copy(stdout, resp.Body); err != nil {
				fmt.Fprintln(stderr, "thesource: reading response:", err)
				return exitSetup
			}
			return exitOK
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintln(stderr, "thesource: reading response:", err)
			return exitSetup
		}
		writeJSON(stdout, body)
		return exitOK
	}
}

func writeJSON(w io.Writer, body []byte) {
	var out bytes.Buffer
	if err := json.Indent(&out, bytes.TrimSpace(body), "", "  "); err != nil {
		w.Write(body)
		return
	}
	out.WriteByte('\n')
	w.Write(out.Bytes())
}

// configPath is thesource.toml beside the real binary, following symlinks so a
// link in ~/.local/bin still finds the config next to the build.
func configPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Join(filepath.Dir(exe), configName), nil
}

func loadBaseURL() (string, error) {
	path, err := configPath()
	if err != nil {
		return "", fmt.Errorf("locate config: %w", err)
	}
	return readBaseURL(path)
}

func readBaseURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read config %s (it should contain: base_url = \"http://host:45068/\"): %w", path, err)
	}
	var c struct {
		BaseURL string `toml:"base_url"`
	}
	if err := toml.Unmarshal(data, &c); err != nil {
		return "", fmt.Errorf("decode config %s: %w", path, err)
	}
	u, err := url.Parse(strings.TrimSpace(c.BaseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("config %s: base_url must be an http(s) URL, got %q", path, c.BaseURL)
	}
	return u.String(), nil
}

func commandLine(c command) string {
	var b strings.Builder
	b.WriteString("thesource " + c.name)
	if c.args != "" {
		b.WriteString(" " + c.args)
	}
	for _, key := range c.params {
		b.WriteString(" [--" + options[key].flag() + " X]")
	}
	if c.ranged {
		b.WriteString(" [--offset N] [--bytes N]")
	}
	if c.retry {
		b.WriteString(" [--retries N]")
	}
	return b.String()
}

func commandHelp(c command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n  %s\n", commandLine(c), c.summary)
	for _, key := range c.params {
		o := options[key]
		fmt.Fprintf(&b, "  --%-11s %s\n", o.flag(), o.usage)
	}
	if c.ranged {
		fmt.Fprintf(&b, "  --%-11s %s\n  --%-11s %s\n", "offset", "start at this byte", "bytes", "return at most this many bytes")
	}
	if c.retry {
		fmt.Fprintf(&b, "  --%-11s %s\n", "retries", "extra attempts when no suitable excerpt is found (default 3)")
	}
	return b.String()
}

func helpText() string {
	var b strings.Builder
	b.WriteString(`thesource - command-line client for The Source (Project Gutenberg books and song lyrics)

COMMANDS
`)
	for _, c := range commands {
		fmt.Fprintf(&b, "  %s\n      %s\n", commandLine(c), c.summary)
	}
	b.WriteString(`  thesource raw /api/v1/PATH?QUERY
      GET any API path and print the JSON (escape hatch for anything above misses)
  thesource config
      Show the config file path and server base URL
  thesource help [books|lyrics|COMMAND]
      This text, or help for one group or command. Also: thesource books,
      thesource books help, thesource books search --help

OPTIONS (passed to the API as-is; flags may come before or after words)
`)
	keys := []string{"language", "available", "limit", "cursor", "year_from", "year_to", "paragraphs", "tag", "views_from", "views_to", "field"}
	for _, key := range keys {
		o := options[key]
		fmt.Fprintf(&b, "  --%-11s %s\n", o.flag(), o.usage)
	}
	b.WriteString(`  --offset/--bytes  byte window of a book text (books text only)
  --retries    extra attempts on no_suitable_excerpt (excerpt commands, default 3)

OUTPUT
  stdout: the API's JSON, indented; "text" commands print plain text.
  Books:    {"id","title","authors":"...","languages":[],"subjects":[],"bookshelves":[],
             "issued","available","original_publication_year"?}
  Search:   {"books"|"songs":[...],"total":N,"next_cursor":"25"}; next_cursor "" = last page.
  Book excerpt:  {"book":{...},"paragraphs":["...", ...]}
  Songs:    {"id","title","artist","views","tag"?,"language"?,"year"?,"features"?}
  Lyrics excerpt: {"song":{...},"lines":["...", ...]}
  Song listings omit lyrics; use "lyrics text ID" for the words.

EXIT STATUS
  0 ok
  1 API error; stderr is {"error":{"code":"...","message":"..."}}. Branch on code:
    invalid_query (fix flags), book_not_found, song_not_found, text_unavailable,
    no_matching_books, no_matching_songs (broaden filters), no_suitable_excerpt,
    lyrics_unavailable (server has no lyrics)
  2 bad command-line usage
  3 config missing/invalid or server unreachable

NOTES
  - Book search matches metadata only, never the text itself. Lyrics search is
    full text over title, artist and lyrics.
  - Books default to English; --language all removes that. Lyrics have no default.
  - Book texts can be megabytes. Start with --bytes 20000, then --offset to read on.
  - Random and excerpt commands pick afresh each call; repeats happen.
  - Publication years come from the text header; books without one are excluded
    whenever --year-from or --year-to is set.

EXAMPLES
  thesource books search pride prejudice --limit 5
  thesource books search dickens --available true
  thesource books get 1342
  thesource books text 1342 --bytes 20000
  thesource books excerpt --paragraphs 2 --year-from 1901
  thesource books random --language fr
  thesource lyrics search concrete jungle
  thesource lyrics search marley --field artist --limit 5
  thesource lyrics text 10
  thesource lyrics excerpt --tag rap
  thesource raw '/api/v1/lyrics/random?views_from=1000000'

CONFIG
  thesource.toml next to the binary (symlinks followed):
    base_url = "http://lemon.com:45068/"
`)
	return b.String()
}

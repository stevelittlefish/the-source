package lyrics

import (
	"path/filepath"
	"reflect"
	"testing"
)

func fixture(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lyrics.db")
	w, err := NewWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []Song{
		{ID: 1, Title: "Blue Skies", Artist: "The Larks", Tag: "pop", Language: "en", Year: 1990, Views: 100, Features: []string{"Guest One"}, Lyrics: "blue skies smiling at me\nnothing but blue skies"},
		{ID: 2, Title: "Concrete Jungle", Artist: "MC Test", Tag: "rap", Language: "en", Year: 2004, Views: 5000, Lyrics: "concrete jungle where dreams are made"},
		{ID: 3, Title: "Ballade", Artist: "Chanteur", Tag: "pop", Language: "fr", Year: 2015, Views: 10, Lyrics: "sous le ciel bleu"},
	} {
		if err := w.Add(s); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Finish(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestGetAndText(t *testing.T) {
	s := fixture(t)
	song, ok, err := s.Get(1)
	if err != nil || !ok {
		t.Fatalf("get: %v %v", ok, err)
	}
	if song.Title != "Blue Skies" || song.Year != 1990 || song.Views != 100 {
		t.Fatalf("song = %+v", song)
	}
	if !reflect.DeepEqual(song.Features, []string{"Guest One"}) {
		t.Fatalf("features = %v", song.Features)
	}
	if song.Lyrics != "" {
		t.Fatal("Get should not carry lyrics")
	}
	body, ok, err := s.Text(1)
	if err != nil || !ok || body == "" {
		t.Fatalf("text: %q %v %v", body, ok, err)
	}
	if _, ok, _ := s.Get(999); ok {
		t.Fatal("phantom song")
	}
}

func TestSearch(t *testing.T) {
	s := fixture(t)
	for _, tc := range []struct {
		filter Filter
		want   int
	}{
		{Filter{Limit: 25}, 3},
		{Filter{Language: "en", Limit: 25}, 2},
		{Filter{Tag: "rap", Limit: 25}, 1},
		{Filter{Query: "blue", Limit: 25}, 1},   // title + lyrics of one song, counted once
		{Filter{Query: "jungle", Limit: 25}, 1}, // lyrics-body match
		{Filter{Query: "larks", Limit: 25}, 1},  // artist match
		{Filter{Query: "larks", Field: "artist", Limit: 25}, 1}, // scoped to the matching column
		{Filter{Query: "larks", Field: "title", Limit: 25}, 0},   // no title carries it
		{Filter{Query: "jungle", Field: "artist", Limit: 25}, 0}, // a body word is not an artist match
		{Filter{Query: "nonexistent", Limit: 25}, 0},
		{Filter{Language: "en", Query: "skies", Limit: 25}, 1},
	} {
		got, total, err := s.Search(tc.filter)
		if err != nil {
			t.Fatalf("%+v: %v", tc.filter, err)
		}
		if total != tc.want || len(got) != tc.want {
			t.Fatalf("%+v: got %d/%d want %d", tc.filter, len(got), total, tc.want)
		}
	}
	// Pagination: total stays whole, page is bounded.
	page, total, err := s.Search(Filter{Limit: 1, Offset: 1})
	if err != nil || total != 3 || len(page) != 1 || page[0].ID != 2 {
		t.Fatalf("page=%+v total=%d err=%v", page, total, err)
	}
}

func TestRandomAndTags(t *testing.T) {
	s := fixture(t)
	if _, ok, err := s.Random(RandomFilter{Language: "fr"}); err != nil || !ok {
		t.Fatalf("random fr: %v %v", ok, err)
	}
	song, ok, err := s.Random(RandomFilter{Tag: "rap"})
	if err != nil || !ok || song.ID != 2 {
		t.Fatalf("random rap = %+v %v %v", song, ok, err)
	}
	if _, ok, _ := s.Random(RandomFilter{Language: "de"}); ok {
		t.Fatal("random matched an absent language")
	}
	// Views bounds: only the rap song (5000 views) clears a 1000 floor.
	if song, ok, err := s.Random(RandomFilter{ViewsFrom: 1000}); err != nil || !ok || song.ID != 2 {
		t.Fatalf("random views>=1000 = %+v %v %v", song, ok, err)
	}
	if _, ok, _ := s.Random(RandomFilter{ViewsFrom: 10000000}); ok {
		t.Fatal("random matched an impossible views floor")
	}
	if song, ok, err := s.Random(RandomFilter{ViewsTo: 50}); err != nil || !ok || song.Views > 50 {
		t.Fatalf("random views<=50 = %+v %v %v", song, ok, err)
	}
	tags, err := s.Tags()
	if err != nil || !reflect.DeepEqual(tags, []string{"pop", "rap"}) {
		t.Fatalf("tags = %v %v", tags, err)
	}
	langs, err := s.Languages()
	if err != nil || !reflect.DeepEqual(langs, []string{"en", "fr"}) {
		t.Fatalf("languages = %v %v", langs, err)
	}
}

func TestParsePGArray(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{`{"Cam\'ron","Opera Steve"}`, []string{"Cam'ron", "Opera Steve"}},
		{`{}`, nil},
		{``, nil},
		{`{NULL}`, nil},
		{`{"He said \"hi\"","plain"}`, []string{`He said "hi"`, "plain"}},
		{`not an array`, nil},
	} {
		if got := ParsePGArray(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ParsePGArray(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func TestCleanYear(t *testing.T) {
	for in, want := range map[int]int{2004: 2004, 2: 0, 0: 0, 3000: 0, 1500: 1500} {
		if got := CleanYear(in); got != want {
			t.Fatalf("CleanYear(%d) = %d, want %d", in, got, want)
		}
	}
}

package server

import (
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"

	"github.com/stevelittlefish/the-source/internal/markov"
)

const maxMarkovCount = 100

// SetChains installs the loaded Markov chains. Call it before serving; the
// chains are read-only afterwards, so requests share them without locks.
func (s *Server) SetChains(chains []*markov.Chain) { s.chains = chains }

func (s *Server) chain(name string) *markov.Chain {
	for _, c := range s.chains {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (s *Server) markovList(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) > 0 {
		fail(w, 400, "invalid_query", "This endpoint takes no query parameters.")
		return
	}
	chains := s.chains
	if chains == nil {
		chains = []*markov.Chain{} // An empty list, not null: nothing configured is still an answer.
	}
	respond(w, 200, struct {
		Chains []*markov.Chain `json:"chains"`
	}{chains})
}

// markovGenerate answers GET /api/v1/markov/{name}?count=N&seed=S with N
// fresh results. The same seed and count always give the same batch.
func (s *Server) markovGenerate(w http.ResponseWriter, r *http.Request, name string) {
	w.Header().Set("Cache-Control", "no-store")
	chain := s.chain(name)
	if chain == nil {
		fail(w, 404, "chain_not_found", "Unknown Markov chain. GET /api/v1/markov lists them.")
		return
	}
	q := r.URL.Query()
	for key, values := range q {
		if (key != "count" && key != "seed") || len(values) != 1 {
			fail(w, 400, "invalid_query", "Use count and seed, each at most once.")
			return
		}
	}
	count := 1
	if q.Has("count") {
		n, err := strconv.Atoi(q.Get("count"))
		if err != nil || n < 1 || n > maxMarkovCount {
			fail(w, 400, "invalid_query", "count must be between 1 and 100.")
			return
		}
		count = n
	}
	var rng *rand.Rand
	var seed *uint64
	if q.Has("seed") {
		n, err := strconv.ParseUint(q.Get("seed"), 10, 64)
		if err != nil {
			fail(w, 400, "invalid_query", "seed must be a non-negative integer.")
			return
		}
		seed = &n
		rng = rand.New(rand.NewPCG(n, n))
	} else {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	results := make([]markov.Result, 0, count)
	for range count {
		result, err := chain.Generate(rng)
		if errors.Is(err, markov.ErrNoResult) {
			fail(w, 503, "no_result", "The chain could not find an original result; try again or use another seed.")
			return
		}
		results = append(results, result)
	}
	respond(w, 200, struct {
		Chain   string          `json:"chain"`
		Kind    string          `json:"kind"`
		Seed    *uint64         `json:"seed,omitempty"`
		Results []markov.Result `json:"results"`
	}{chain.Name, chain.Kind, seed, results})
}

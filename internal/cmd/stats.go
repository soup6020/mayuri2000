package cmd

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Invocation describes how a command was invoked.
type Invocation int

const (
	InvokedSlash Invocation = iota
	InvokedPrefix
	InvokedMenu
)

// Stats tracks command usage since the bot started.
type Stats struct {
	Start time.Time

	Slash, Prefix, Menu atomic.Int64
	Errors              atomic.Int64
	ImageJobs           atomic.Int64

	mu     sync.Mutex
	counts map[string]int64
}

func newStats() *Stats {
	return &Stats{Start: time.Now(), counts: map[string]int64{}}
}

func (s *Stats) record(name string, kind Invocation, failed bool) {
	switch kind {
	case InvokedSlash:
		s.Slash.Add(1)
	case InvokedPrefix:
		s.Prefix.Add(1)
	case InvokedMenu:
		s.Menu.Add(1)
	}
	if failed {
		s.Errors.Add(1)
	}
	s.mu.Lock()
	s.counts[name]++
	s.mu.Unlock()
}

// Count is a command usage count.
type Count struct {
	Name  string
	Count int64
}

// Total returns the number of commands run since start.
func (s *Stats) Total() int64 { return s.Slash.Load() + s.Prefix.Load() + s.Menu.Load() }

// Top returns the n most used commands since start.
func (s *Stats) Top(n int) []Count {
	s.mu.Lock()
	out := make([]Count, 0, len(s.counts))
	for name, c := range s.counts {
		out = append(out, Count{name, c})
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

package search

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"mayuri2000/internal/cmd"
)

const frogTipsURL = "https://frog.tips/api/1/tips/"

// frogTips hands out tips from a cached "croak" (a batch of ~50 random tips
// from frog.tips), fetching a new batch only when it runs out.
type frogTips struct {
	mu   sync.Mutex
	tips []string
}

// ParseFrogTips reads a croak from the frog.tips API.
func ParseFrogTips(data []byte) ([]string, error) {
	var croak struct {
		Tips []struct {
			Tip string `json:"tip"`
		} `json:"tips"`
	}
	if err := json.Unmarshal(data, &croak); err != nil {
		return nil, err
	}
	var tips []string
	for _, t := range croak.Tips {
		if tip := strings.TrimSpace(t.Tip); tip != "" {
			tips = append(tips, tip)
		}
	}
	return tips, nil
}

func (f *frogTips) next(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.tips) == 0 {
		data, err := get(ctx, frogTipsURL)
		if err != nil {
			return "", err
		}
		if f.tips, err = ParseFrogTips(data); err != nil {
			return "", err
		}
		if len(f.tips) == 0 {
			return "", cmd.Errorf("FROG.TIPS HAS NO TIPS RIGHT NOW. PLEASE CONSULT YOUR FROG MANUAL.")
		}
	}
	tip := f.tips[0]
	f.tips = f.tips[1:]
	return tip, nil
}

func (s *Searcher) frogTip(c *cmd.Context) error {
	tip, err := s.frog.next(c)
	if err != nil {
		var ue *cmd.UserError
		if errors.As(err, &ue) {
			return err
		}
		return cmd.Errorf("frog.tips isn't responding right now.")
	}
	return c.Reply("🐸 " + tip)
}

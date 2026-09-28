package cmd

import (
	"context"

	"github.com/disgoorg/disgo/discord"
)

// Recorder captures messages sent by a command, for tests.
type Recorder struct{ Messages []discord.MessageCreate }

func (r *Recorder) deferReply(bool) error { return nil }
func (r *Recorder) send(m discord.MessageCreate) error {
	r.Messages = append(r.Messages, m)
	return nil
}
func (r *Recorder) done() {}

// NewTestContext builds a Context whose replies are captured by the returned
// Recorder. args are option values keyed by option name.
func NewTestContext(reg *Registry, args map[string]any) (*Context, *Recorder) {
	rec := &Recorder{}
	if args == nil {
		args = map[string]any{}
	}
	return &Context{Context: context.Background(), Registry: reg, Prefix: "/", args: args, resp: rec}, rec
}

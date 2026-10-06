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

// RunPrefix runs a command as if invoked with the text prefix "%" and the
// given argument text, using the same argument binding as real messages.
// It returns the recorder and the command's error (nil if the arguments
// were rejected; the rejection reply is recorded instead).
func RunPrefix(reg *Registry, command *Command, argText string) (*Recorder, error) {
	c, rec := NewTestContext(reg, nil)
	c.Prefix = "%"
	c.SourceMessage = &discord.Message{}
	c.Invocation = InvokedPrefix
	if problem := reg.bindPrefixArgs(c, command, "%", argText, nil); problem != "" {
		_ = c.Reply(problem)
		return rec, nil
	}
	return rec, command.Run(c)
}

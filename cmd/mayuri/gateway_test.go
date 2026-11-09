package main

import (
	"encoding/json"
	"testing"

	"github.com/disgoorg/disgo/gateway"
)

// Discord adds channel types without notice. disgo v0.19 failed the whole GUILD_CREATE on an
// unknown type (type 20, "server-hub", in Nov 2026), so the gateway reconnected in a loop
// until Discord rejected the session. Guard against regressing to a version that does that.
func TestGuildCreateWithUnknownChannelType(t *testing.T) {
	payload := `{
		"id": "1", "name": "test", "unavailable": false,
		"channels": [
			{"id": "2", "type": 0, "name": "general", "position": 0, "permission_overwrites": []},
			{"id": "3", "type": 20, "name": "server-hub", "position": 0, "permission_overwrites": [], "parent_id": null, "flags": 0},
			{"id": "4", "type": 9999, "name": "future", "position": 1, "permission_overwrites": []}
		]
	}`
	var ev gateway.EventGuildCreate
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		t.Fatalf("GUILD_CREATE with an unknown channel type failed to decode: %v", err)
	}
	if len(ev.Channels) != 3 {
		t.Fatalf("got %d channels, want 3", len(ev.Channels))
	}
	if got := ev.Channels[1].Name(); got != "server-hub" {
		t.Errorf("unknown channel name = %q, want server-hub", got)
	}
}

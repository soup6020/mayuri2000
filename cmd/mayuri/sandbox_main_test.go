package main

import (
	"testing"

	"mayuri2000/internal/sandbox"
)

// The test binary doubles as the decoder sandbox launcher.
func TestMain(m *testing.M) { sandbox.RunTests(m) }

package main

import (
	"strings"
	"testing"
)

// TestBuildCommandCatalog verifies the doc tool can pull the full command
// catalog straight from the live registry — no server boot — and that admin
// verbs are surfaced (this is an authoring reference, not the player `help`).
func TestBuildCommandCatalog(t *testing.T) {
	cats, err := buildCommandCatalog()
	if err != nil {
		t.Fatalf("buildCommandCatalog: %v", err)
	}
	if len(cats) == 0 {
		t.Fatal("expected at least one command category")
	}
	sawAdmin := false
	for _, c := range cats {
		if c.Key == "admin" {
			sawAdmin = true
		}
		if len(c.Commands) == 0 {
			t.Errorf("category %q has no commands", c.Key)
		}
	}
	if !sawAdmin {
		t.Error("expected admin category to be surfaced in the command catalog")
	}
}

// TestRenderCommands locks the page shape: a per-category anchor + a table with
// the Command/Usage/Description columns, driven by the real registry.
func TestRenderCommands(t *testing.T) {
	body, err := renderCommands()
	if err != nil {
		t.Fatalf("renderCommands: %v", err)
	}
	for _, want := range []string{
		`<h2 id="general">`,
		`<h2 id="combat">`,
		`<h2 id="admin">`,
		"<th>Command</th>",
		"<th>Usage</th>",
		"<th>Description</th>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered commands missing %q", want)
		}
	}
}

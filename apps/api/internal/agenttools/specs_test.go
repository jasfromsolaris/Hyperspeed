package agenttools

import (
	"strings"
	"testing"
)

func TestAgentToolSpecsIncludesCreateText(t *testing.T) {
	specs := AgentToolSpecs()
	var names []string
	for _, s := range specs {
		names = append(names, s["name"].(string))
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "space.file.create_text") {
		t.Fatalf("missing create_text in %s", got)
	}
	if !strings.Contains(got, "space.folder.create") {
		t.Fatalf("missing folder.create in %s", got)
	}
}

func TestHyperspeedFunctionToolsOpenRouterJSON(t *testing.T) {
	raw, err := HyperspeedFunctionToolsOpenRouterJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != len(AgentToolSpecs()) {
		t.Fatalf("len %d vs specs %d", len(raw), len(AgentToolSpecs()))
	}
}

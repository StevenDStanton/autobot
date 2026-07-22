package main

import (
	"encoding/json"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/thesimpledev/autobotgo/internal/config"
	"github.com/thesimpledev/autobotgo/internal/stage"
)

// update regenerates config.example.json instead of checking it:
//
//	go test . -update
var update = flag.Bool("update", false, "rewrite config.example.json from the built-in defaults")

const exampleConfigPath = "config.example.json"

// TestConfigExampleMatchesDefaults keeps the committed example config in
// step with the schema. The example is what a reader sees before they run
// anything, so a stale one teaches them a config that no longer loads.
func TestConfigExampleMatchesDefaults(t *testing.T) {
	want, err := json.MarshalIndent(config.Default(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')

	if *update {
		if err := os.WriteFile(exampleConfigPath, want, 0o644); err != nil { // #nosec G306 -- example holds placeholders, not secrets
			t.Fatal(err)
		}
		t.Logf("wrote %s", exampleConfigPath)
		return
	}

	got, err := os.ReadFile(exampleConfigPath)
	if err != nil {
		t.Fatalf("reading %s: %v", exampleConfigPath, err)
	}
	if string(got) != string(want) {
		t.Errorf("%s is out of date with config.Default(); run `go test . -update` to regenerate",
			exampleConfigPath)
	}
}

// TestConfigExampleLoads proves the committed example is a config the
// binary actually accepts, so `--config config.example.json` works.
func TestConfigExampleLoads(t *testing.T) {
	cfg, err := config.Load(exampleConfigPath)
	if err != nil {
		t.Fatalf("the committed example config does not load: %v", err)
	}
	// It must ship with placeholders, never real credentials.
	if err := cfg.CheckSecrets(true, true, false); err == nil {
		t.Error("the example config passed the secret check, which means it contains real values")
	}
}

func TestSelectStages(t *testing.T) {
	pipeline := stage.Pipeline(true)

	t.Run("empty selection returns the whole pipeline", func(t *testing.T) {
		got, err := selectStages(pipeline, "")
		if err != nil {
			t.Fatalf("selectStages: %v", err)
		}
		if len(got) != len(pipeline) {
			t.Errorf("got %d stages, want all %d", len(got), len(pipeline))
		}
	})

	t.Run("blank selection returns the whole pipeline", func(t *testing.T) {
		got, err := selectStages(pipeline, "   ")
		if err != nil {
			t.Fatalf("selectStages: %v", err)
		}
		if len(got) != len(pipeline) {
			t.Errorf("got %d stages, want all %d", len(got), len(pipeline))
		}
	})

	t.Run("pipeline order wins over the order given", func(t *testing.T) {
		got, err := selectStages(pipeline, stage.NameVideo+","+stage.NameStory)
		if err != nil {
			t.Fatalf("selectStages: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d stages, want 2", len(got))
		}
		if got[0].Name != stage.NameStory || got[1].Name != stage.NameVideo {
			t.Errorf("got %s,%s, want %s,%s", got[0].Name, got[1].Name, stage.NameStory, stage.NameVideo)
		}
	})

	t.Run("surrounding spaces are tolerated", func(t *testing.T) {
		got, err := selectStages(pipeline, " story , video ")
		if err != nil {
			t.Fatalf("selectStages: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d stages, want 2", len(got))
		}
	})

	t.Run("unknown stage is an error", func(t *testing.T) {
		_, err := selectStages(pipeline, "story,nope")
		if err == nil {
			t.Fatal("selectStages accepted an unknown stage name")
		}
	})
}

func TestSecretNeeds(t *testing.T) {
	tests := []struct {
		name                      string
		stages                    []string
		openai, youtube, awsCreds bool
	}{
		{name: "story needs openai only", stages: []string{stage.NameStory}, openai: true},
		{name: "images need openai only", stages: []string{stage.NameImages}, openai: true},
		{name: "upload needs youtube only", stages: []string{stage.NameUpload}, youtube: true},
		{name: "archive needs aws only", stages: []string{stage.NameArchive}, awsCreds: true},
		{name: "video needs nothing", stages: []string{stage.NameVideo}},
		{name: "cleanup needs nothing", stages: []string{stage.NameCleanup}},
		{
			name:   "full pipeline needs all three",
			stages: []string{stage.NameStory, stage.NameUpload, stage.NameArchive},
			openai: true, youtube: true, awsCreds: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := selectStages(stage.Pipeline(false), strings.Join(tc.stages, ","))
			if err != nil {
				t.Fatalf("selectStages: %v", err)
			}
			openai, youtube, aws := secretNeeds(selected)
			if openai != tc.openai || youtube != tc.youtube || aws != tc.awsCreds {
				t.Errorf("secretNeeds = (openai=%v youtube=%v aws=%v), want (openai=%v youtube=%v aws=%v)",
					openai, youtube, aws, tc.openai, tc.youtube, tc.awsCreds)
			}
		})
	}
}

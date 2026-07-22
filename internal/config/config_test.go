package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConfig writes body to a temp config.json and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultIsValid(t *testing.T) {
	// The template `autobotgo init` writes must itself pass validation,
	// otherwise a new user's first run fails on config they never touched.
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default() does not validate: %v", err)
	}
}

func TestLoad(t *testing.T) {
	t.Run("absent fields keep their defaults", func(t *testing.T) {
		path := writeConfig(t, `{"run_time": "09:30"}`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.RunTime != "09:30" {
			t.Errorf("run_time = %q, want %q", cfg.RunTime, "09:30")
		}
		if cfg.Timezone != Default().Timezone {
			t.Errorf("timezone = %q, want the default %q", cfg.Timezone, Default().Timezone)
		}
		if cfg.Video.FPS != Default().Video.FPS {
			t.Errorf("video.fps = %d, want the default %d", cfg.Video.FPS, Default().Video.FPS)
		}
	})

	t.Run("unknown fields are rejected", func(t *testing.T) {
		// A typo in a key must be loud. Silently ignoring it would mean a
		// setting the user believes is applied simply is not.
		path := writeConfig(t, `{"run_tim": "09:30"}`)
		_, err := Load(path)
		if err == nil {
			t.Fatal("Load accepted an unknown field, want an error")
		}
		if !strings.Contains(err.Error(), "run_tim") {
			t.Errorf("error %q does not name the offending field", err)
		}
	})

	t.Run("base dir and config path are absolute", func(t *testing.T) {
		path := writeConfig(t, `{}`)
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !filepath.IsAbs(cfg.BaseDir) || !filepath.IsAbs(cfg.ConfigPath) {
			t.Errorf("BaseDir = %q, ConfigPath = %q, both should be absolute", cfg.BaseDir, cfg.ConfigPath)
		}
		if cfg.BaseDir != filepath.Dir(path) {
			t.Errorf("BaseDir = %q, want %q", cfg.BaseDir, filepath.Dir(path))
		}
	})

	t.Run("missing file mentions init", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "nope.json"))
		if err == nil {
			t.Fatal("Load of a missing file returned nil error")
		}
		if !strings.Contains(err.Error(), "autobotgo init") {
			t.Errorf("error %q should point the user at `autobotgo init`", err)
		}
	})

	t.Run("malformed json is rejected", func(t *testing.T) {
		path := writeConfig(t, `{"run_time":`)
		if _, err := Load(path); err == nil {
			t.Fatal("Load accepted malformed JSON")
		}
	})

	t.Run("invalid values are rejected", func(t *testing.T) {
		path := writeConfig(t, `{"run_time": "25:00"}`)
		if _, err := Load(path); err == nil {
			t.Fatal("Load accepted an out-of-range run_time")
		}
	})
}

func TestValidateTimezone(t *testing.T) {
	tests := []struct {
		name    string
		tz      string
		wantErr bool
	}{
		{name: "iana name", tz: "America/New_York"},
		{name: "utc", tz: "UTC"},
		{name: "other continent", tz: "Europe/Berlin"},
		{name: "empty", tz: "", wantErr: true},
		{name: "not a timezone", tz: "Mars/Olympus_Mons", wantErr: true},
		{name: "abbreviation is not an iana name", tz: "EST5EDT4", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.Timezone = tc.tz
			err := cfg.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate accepted timezone %q", tc.tz)
				}
				if !strings.Contains(err.Error(), "timezone") {
					t.Errorf("error %q does not mention timezone", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate rejected timezone %q: %v", tc.tz, err)
			}
			if cfg.Location() == nil {
				t.Fatal("Location() is nil after a successful Validate")
			}
			if cfg.Location().String() != tc.tz {
				t.Errorf("Location() = %q, want %q", cfg.Location(), tc.tz)
			}
		})
	}
}

func TestLocationFallsBackToUTC(t *testing.T) {
	// A config built by hand rather than through Load has no resolved
	// location, and callers must still get a usable one.
	var cfg Config
	if cfg.Location() != time.UTC {
		t.Errorf("Location() = %v, want UTC for an unvalidated config", cfg.Location())
	}
}

func TestValidateCollectsEveryProblem(t *testing.T) {
	cfg := Default()
	cfg.OpenAI.TextModel = ""
	cfg.Video.FPS = 500
	cfg.RunTime = "nope"

	err := cfg.Validate()
	if err != nil {
		msg := err.Error()
		for _, want := range []string{"openai.text_model", "video.fps", "run_time"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error is missing %q; user should see all problems at once:\n%s", want, msg)
			}
		}
		return
	}
	t.Fatal("Validate accepted a config with three problems")
}

func TestValidateRunDays(t *testing.T) {
	tests := []struct {
		name    string
		days    []string
		wantErr bool
	}{
		{name: "empty means every day", days: nil},
		{name: "lowercase names", days: []string{"monday", "friday"}},
		{name: "mixed case is tolerated", days: []string{"Monday"}},
		{name: "not a weekday", days: []string{"caturday"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			cfg.RunDays = tc.days
			err := cfg.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestIsRunDay(t *testing.T) {
	tests := []struct {
		name string
		days []string
		day  time.Weekday
		want bool
	}{
		{name: "empty list runs every day", days: nil, day: time.Tuesday, want: true},
		{name: "listed day runs", days: []string{"monday", "friday"}, day: time.Friday, want: true},
		{name: "unlisted day does not", days: []string{"monday", "friday"}, day: time.Tuesday, want: false},
		{name: "case insensitive", days: []string{"MONDAY"}, day: time.Monday, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{RunDays: tc.days}
			if got := cfg.IsRunDay(tc.day); got != tc.want {
				t.Errorf("IsRunDay(%v) = %v, want %v", tc.day, got, tc.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	cfg := &Config{BaseDir: "/opt/autobotgo"}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "relative resolves against base", in: "runs", want: "/opt/autobotgo/runs"},
		{name: "nested relative", in: "a/b.txt", want: "/opt/autobotgo/a/b.txt"},
		{name: "absolute is untouched", in: "/var/lib/x", want: "/var/lib/x"},
		{name: "empty stays empty", in: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := cfg.Resolve(tc.in); got != tc.want {
				t.Errorf("Resolve(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStoryModelName(t *testing.T) {
	t.Run("falls back to the text model", func(t *testing.T) {
		o := &OpenAI{TextModel: "text-model"}
		if got := o.StoryModelName(); got != "text-model" {
			t.Errorf("StoryModelName() = %q, want %q", got, "text-model")
		}
	})
	t.Run("prefers the story model", func(t *testing.T) {
		o := &OpenAI{TextModel: "text-model", StoryModel: "story-model"}
		if got := o.StoryModelName(); got != "story-model" {
			t.Errorf("StoryModelName() = %q, want %q", got, "story-model")
		}
	})
}

func TestFilled(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "real value", in: "sk-abc123", want: true},
		{name: "empty", in: "", want: false},
		{name: "the template placeholder", in: Placeholder, want: false},
		{name: "any run of asterisks", in: "****", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := filled(tc.in); got != tc.want {
				t.Errorf("filled(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCheckSecretsAWS(t *testing.T) {
	base := func() *Config {
		cfg := Default()
		cfg.AWS.Enabled = true
		cfg.AWS.Bucket = "my-bucket"
		cfg.AWS.Region = "us-east-1"
		return cfg
	}

	tests := []struct {
		name     string
		mutate   func(*Config)
		wantErr  bool
		errMatch string
	}{
		{
			name:   "both keys set is allowed",
			mutate: func(c *Config) { c.AWS.AccessKeyID = "AKIA"; c.AWS.SecretAccessKey = "secret" },
		},
		{
			name: "custom endpoint with keys is allowed",
			mutate: func(c *Config) {
				c.AWS.Endpoint = "https://nyc3.digitaloceanspaces.com"
				c.AWS.AccessKeyID = "DO00"
				c.AWS.SecretAccessKey = "secret"
			},
		},
		{
			name:     "missing keys are rejected",
			mutate:   func(*Config) {},
			wantErr:  true,
			errMatch: "access_key_id",
		},
		{
			name:     "id without secret is rejected",
			mutate:   func(c *Config) { c.AWS.AccessKeyID = "AKIA" },
			wantErr:  true,
			errMatch: "secret_access_key",
		},
		{
			name:     "missing bucket is rejected",
			mutate:   func(c *Config) { c.AWS.Bucket = "" },
			wantErr:  true,
			errMatch: "aws.bucket",
		},
		{
			name:     "missing region is rejected",
			mutate:   func(c *Config) { c.AWS.Region = "" },
			wantErr:  true,
			errMatch: "aws.region",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.mutate(cfg)
			err := cfg.CheckSecrets(false, false, true)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("CheckSecrets returned %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("CheckSecrets returned nil, want an error")
			}
			if !strings.Contains(err.Error(), tc.errMatch) {
				t.Errorf("error %q does not mention %q", err, tc.errMatch)
			}
		})
	}

	t.Run("disabled aws needs nothing", func(t *testing.T) {
		cfg := Default() // aws.enabled is false
		if err := cfg.CheckSecrets(false, false, true); err != nil {
			t.Errorf("CheckSecrets returned %v for disabled aws, want nil", err)
		}
	})

	t.Run("not needed means not checked", func(t *testing.T) {
		cfg := base()
		cfg.AWS.Bucket = ""
		if err := cfg.CheckSecrets(false, false, false); err != nil {
			t.Errorf("CheckSecrets returned %v when aws was not needed, want nil", err)
		}
	})
}

func TestCheckSecretsOpenAIAndYouTube(t *testing.T) {
	t.Run("template placeholders count as missing", func(t *testing.T) {
		cfg := Default() // every secret is the placeholder
		err := cfg.CheckSecrets(true, true, false)
		if err == nil {
			t.Fatal("CheckSecrets accepted the placeholder template")
		}
		for _, want := range []string{"openai.api_key", "youtube.client_id", "youtube.refresh_token"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error is missing %q:\n%s", want, err)
			}
		}
	})

	t.Run("filled secrets pass", func(t *testing.T) {
		cfg := Default()
		cfg.OpenAI.APIKey = "sk-test"
		cfg.YouTube.ClientID = "id"
		cfg.YouTube.ClientSecret = "secret"
		cfg.YouTube.RefreshToken = "token"
		if err := cfg.CheckSecrets(true, true, false); err != nil {
			t.Errorf("CheckSecrets returned %v, want nil", err)
		}
	})

	t.Run("disabled youtube needs no youtube secrets", func(t *testing.T) {
		cfg := Default()
		cfg.OpenAI.APIKey = "sk-test"
		cfg.YouTube.Enabled = false
		if err := cfg.CheckSecrets(true, true, false); err != nil {
			t.Errorf("CheckSecrets returned %v, want nil", err)
		}
	})

	t.Run("refresh token is not required by CheckYouTubeClient", func(t *testing.T) {
		// `autobotgo auth` produces the refresh token, so demanding it
		// before the auth flow would be a deadlock.
		cfg := Default()
		cfg.YouTube.ClientID = "id"
		cfg.YouTube.ClientSecret = "secret"
		if err := cfg.CheckYouTubeClient(); err != nil {
			t.Errorf("CheckYouTubeClient returned %v, want nil", err)
		}
	})
}

func TestSaveAndWriteTemplate(t *testing.T) {
	t.Run("save round-trips through load", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		cfg := Default()
		cfg.YouTube.RefreshToken = "written-back"
		if err := cfg.Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := Load(path)
		if err != nil {
			t.Fatalf("Load after Save: %v", err)
		}
		if got.YouTube.RefreshToken != "written-back" {
			t.Errorf("refresh_token = %q, want %q", got.YouTube.RefreshToken, "written-back")
		}
	})

	t.Run("save uses owner-only permissions", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := Default().Save(path); err != nil {
			t.Fatalf("Save: %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		// The file holds an API key and a refresh token.
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissions = %o, want 600", perm)
		}
	})

	t.Run("write template refuses to overwrite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := WriteTemplate(path); err != nil {
			t.Fatalf("WriteTemplate: %v", err)
		}
		err := WriteTemplate(path)
		if err == nil {
			t.Fatal("WriteTemplate overwrote an existing config")
		}
		if !strings.Contains(err.Error(), "not overwriting") {
			t.Errorf("error %q should say it is not overwriting", err)
		}
	})

	t.Run("template loads cleanly", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := WriteTemplate(path); err != nil {
			t.Fatalf("WriteTemplate: %v", err)
		}
		if _, err := Load(path); err != nil {
			t.Fatalf("the written template does not load: %v", err)
		}
	})
}

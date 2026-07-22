// Package config defines the autobotgo configuration file, its defaults,
// validation, and the template written by `autobotgo init`.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type OpenAI struct {
	APIKey string `json:"api_key"`
	// TextModel handles the utility calls (world bible, metadata, scene
	// prompts, story audits). StoryModel writes and revises the story
	// itself, typically a slower reasoning model; empty means use
	// TextModel.
	TextModel        string `json:"text_model"`
	StoryModel       string `json:"story_model"`
	TTSModel         string `json:"tts_model"`
	TTSVoice         string `json:"tts_voice"`
	TTSInstructions  string `json:"tts_instructions"`
	TTSMaxChunkTok   int    `json:"tts_max_chunk_tokens"`
	ImageModel       string `json:"image_model"`
	ImageSize        string `json:"image_size"`
	ImageQuality     string `json:"image_quality"`
	ImageStylePrompt string `json:"image_style_prompt"`
}

type Story struct {
	PromptFile     string `json:"prompt_file"`
	TargetMinutes  int    `json:"target_minutes"`
	TargetWordsMin int    `json:"target_words_min"`
	TargetWordsMax int    `json:"target_words_max"`
}

// Clips is a placeholder seam for mixing generated video clips (e.g. Veo)
// into the timeline later. Not implemented.
type Clips struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
}

type Video struct {
	ImagesPerMinute  float64 `json:"images_per_minute"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	FPS              int     `json:"fps"`
	CrossfadeSeconds float64 `json:"crossfade_seconds"`
	KenBurnsMaxZoom  float64 `json:"kenburns_max_zoom"`
	EndHoldSeconds   float64 `json:"end_hold_seconds"`
	X264Preset       string  `json:"x264_preset"`
	X264CRF          int     `json:"x264_crf"`
	BackgroundMusic  string  `json:"background_music_path"`
	MusicVolume      float64 `json:"music_volume"`
	Clips            Clips   `json:"clips"`
}

type YouTube struct {
	Enabled bool `json:"enabled"`
	// ClientID and ClientSecret come from the OAuth Desktop-app client
	// in Google Cloud Console. RefreshToken is written by `autobotgo auth`.
	ClientID      string   `json:"client_id"`
	ClientSecret  string   `json:"client_secret"`
	RefreshToken  string   `json:"refresh_token"`
	PrivacyStatus string   `json:"privacy_status"`
	CategoryID    string   `json:"category_id"`
	MadeForKids   bool     `json:"made_for_kids"`
	ExtraTags     []string `json:"extra_tags"`
	PlaylistID    string   `json:"playlist_id"`
}

// World controls the self-referencing story world: a capped world.md
// canon file plus optional retrieval over the full story archive.
type World struct {
	// MaxWords caps world.md; when an update pushes it over, the model
	// compresses it back down to ~70% of the cap.
	MaxWords int `json:"max_words"`
	// RAGEnabled uploads each story to an OpenAI vector store and lets
	// story generation search the archive.
	RAGEnabled bool `json:"rag_enabled"`
	// VectorStoreID is created automatically on the first run and saved
	// back into this file. Leave it empty.
	VectorStoreID string `json:"vector_store_id"`
}

type AWS struct {
	Enabled bool   `json:"enabled"`
	Region  string `json:"region"`
	Bucket  string `json:"bucket"`
	Prefix  string `json:"prefix"`
	// Endpoint overrides the S3 endpoint for S3-compatible stores such as
	// DigitalOcean Spaces, MinIO, Backblaze B2, and Cloudflare R2, for
	// example "https://nyc3.digitaloceanspaces.com". Leave it empty for
	// real AWS S3.
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

type Config struct {
	OpenAI  OpenAI  `json:"openai"`
	Story   Story   `json:"story"`
	World   World   `json:"world"`
	Video   Video   `json:"video"`
	YouTube YouTube `json:"youtube"`
	AWS     AWS     `json:"aws"`
	// RunTime is the "HH:MM" the scheduler fires, interpreted in Timezone.
	// RunDays limits which weekdays it fires on (lowercase day names,
	// e.g. ["monday","wednesday","friday"]); empty means every day. The
	// test-run trigger file works regardless.
	//
	// Timezone is an IANA name such as "America/New_York" or "UTC". The
	// server's own timezone is deliberately ignored, so moving the server
	// never moves the publish time.
	RunTime   string   `json:"run_time"`
	RunDays   []string `json:"run_days"`
	Timezone  string   `json:"timezone"`
	RunsDir   string   `json:"runs_dir"`
	NotifyURL string   `json:"notify_url"`
	LogFile   string   `json:"log_file"`

	// BaseDir is the directory the config file lives in; relative paths
	// resolve against it and the typed artifact folders live here.
	// ConfigPath is the file itself, for saving values back (refresh
	// token, vector store id).
	BaseDir    string `json:"-"`
	ConfigPath string `json:"-"`

	// loc is Timezone resolved once by Validate; read it via Location.
	loc *time.Location
}

// Location returns the resolved scheduler timezone. Validate fills it in,
// so a config that came from Load always has one.
func (c *Config) Location() *time.Location {
	if c.loc == nil {
		return time.UTC
	}
	return c.loc
}

// Placeholder marks values the user must fill in. It is visually loud in
// the generated template, and validation treats it the same as empty.
const Placeholder = "************************"

// filled reports whether a required value was actually provided (not
// empty, not a run of placeholder asterisks).
func filled(s string) bool {
	return s != "" && strings.Trim(s, "*") != ""
}

// Default returns a fully populated config with sensible defaults and
// loud placeholders for the values only the user can supply. It is both
// the starting point for Load (so absent fields keep their defaults) and
// the body of the template written by `autobotgo init`.
func Default() *Config {
	return &Config{
		OpenAI: OpenAI{
			APIKey:           Placeholder,
			TextModel:        "gpt-5.4",
			TTSModel:         "gpt-4o-mini-tts",
			TTSVoice:         "onyx",
			TTSInstructions:  "Warm, unhurried storyteller. Calm pacing, gentle emphasis, slight pauses between paragraphs.",
			TTSMaxChunkTok:   1500,
			ImageModel:       "gpt-image-1.5",
			ImageSize:        "1536x1024",
			ImageQuality:     "high",
			ImageStylePrompt: "Soft painterly digital illustration, muted warm palette, cinematic lighting, no text or captions",
		},
		Story: Story{
			PromptFile:     "story.md",
			TargetMinutes:  5,
			TargetWordsMin: 750,
			TargetWordsMax: 850,
		},
		Video: Video{
			ImagesPerMinute:  2.0,
			Width:            1920,
			Height:           1080,
			FPS:              30,
			CrossfadeSeconds: 1.0,
			KenBurnsMaxZoom:  1.12,
			EndHoldSeconds:   2.0,
			X264Preset:       "medium",
			X264CRF:          18,
			MusicVolume:      0.12,
		},
		YouTube: YouTube{
			Enabled:       true,
			ClientID:      Placeholder,
			ClientSecret:  Placeholder,
			RefreshToken:  Placeholder,
			PrivacyStatus: "private",
			CategoryID:    "22",
			ExtraTags:     []string{},
		},
		AWS: AWS{
			Enabled: false,
			Region:  "us-east-1",
			Prefix:  "autobotgo/",
		},
		World: World{
			MaxWords:   4000,
			RAGEnabled: true,
		},
		RunTime:  "08:00",
		Timezone: "America/New_York",
		RunsDir:  "runs",
		LogFile:  "autobotgo.log",
	}
}

// Load reads and structurally validates the config at path. Secret
// requirements depend on which stages will run; callers check those
// separately with CheckSecrets.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from the -config flag
	if err != nil {
		return nil, fmt.Errorf("reading config: %w (run `autobotgo init` to create a template)", err)
	}
	cfg := Default()
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	cfg.BaseDir = filepath.Dir(abs)
	cfg.ConfigPath = abs
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// IsRunDay reports whether the scheduler fires on the given weekday.
// An empty run_days list means every day.
func (c *Config) IsRunDay(day time.Weekday) bool {
	if len(c.RunDays) == 0 {
		return true
	}
	name := strings.ToLower(day.String())
	for _, d := range c.RunDays {
		if strings.ToLower(d) == name {
			return true
		}
	}
	return false
}

// StoryModelName returns the model that writes and revises stories,
// falling back to the general text model when unset.
func (o *OpenAI) StoryModelName() string {
	if o.StoryModel != "" {
		return o.StoryModel
	}
	return o.TextModel
}

// Resolve makes a config-relative path absolute against BaseDir.
func (c *Config) Resolve(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.BaseDir, p)
}

// Validate collects every structural problem so the user can fix them all
// at once. Secrets are checked separately by CheckSecrets.
func (c *Config) Validate() error {
	var probs []string
	add := func(format string, args ...any) {
		probs = append(probs, fmt.Sprintf(format, args...))
	}

	if c.OpenAI.TextModel == "" {
		add("openai.text_model is empty")
	}
	if c.OpenAI.TTSModel == "" {
		add("openai.tts_model is empty")
	}
	if c.OpenAI.TTSVoice == "" {
		add("openai.tts_voice is empty")
	}
	if c.OpenAI.TTSMaxChunkTok < 100 {
		add("openai.tts_max_chunk_tokens must be at least 100 (got %d)", c.OpenAI.TTSMaxChunkTok)
	}
	if c.OpenAI.ImageModel == "" {
		add("openai.image_model is empty")
	}

	if c.Story.TargetMinutes < 1 {
		add("story.target_minutes must be at least 1 (got %d)", c.Story.TargetMinutes)
	}
	if c.Story.TargetWordsMin < 50 || c.Story.TargetWordsMax < c.Story.TargetWordsMin {
		add("story.target_words_min/max must satisfy 50 <= min <= max (got %d/%d)",
			c.Story.TargetWordsMin, c.Story.TargetWordsMax)
	}

	v := c.Video
	if v.ImagesPerMinute <= 0 {
		add("video.images_per_minute must be positive (got %g)", v.ImagesPerMinute)
	}
	if v.Width < 320 || v.Height < 240 {
		add("video.width/height too small (got %dx%d)", v.Width, v.Height)
	}
	if v.FPS < 1 || v.FPS > 120 {
		add("video.fps must be 1-120 (got %d)", v.FPS)
	}
	if v.CrossfadeSeconds < 0 {
		add("video.crossfade_seconds must not be negative (got %g)", v.CrossfadeSeconds)
	}
	if v.KenBurnsMaxZoom < 1.0 || v.KenBurnsMaxZoom > 2.0 {
		add("video.kenburns_max_zoom must be 1.0-2.0 (got %g)", v.KenBurnsMaxZoom)
	}
	if v.X264CRF < 0 || v.X264CRF > 51 {
		add("video.x264_crf must be 0-51 (got %d)", v.X264CRF)
	}
	if v.MusicVolume < 0 || v.MusicVolume > 1 {
		add("video.music_volume must be 0-1 (got %g)", v.MusicVolume)
	}
	if v.BackgroundMusic != "" {
		if _, err := os.Stat(c.Resolve(v.BackgroundMusic)); err != nil {
			add("video.background_music_path: %v", err)
		}
	}
	if v.Clips.Enabled {
		add("video.clips.enabled is set but clip mixing is not implemented yet")
	}

	if c.YouTube.Enabled {
		switch c.YouTube.PrivacyStatus {
		case "private", "unlisted", "public":
		default:
			add("youtube.privacy_status must be private, unlisted, or public (got %q)", c.YouTube.PrivacyStatus)
		}
	}

	if c.World.MaxWords < 500 {
		add("world.max_words must be at least 500 (got %d)", c.World.MaxWords)
	}

	if _, err := time.Parse("15:04", c.RunTime); err != nil {
		add("run_time must be HH:MM 24-hour format, e.g. \"08:00\" (got %q)", c.RunTime)
	}
	switch loc, err := time.LoadLocation(c.Timezone); {
	case c.Timezone == "":
		add("timezone is empty (use an IANA name such as \"America/New_York\" or \"UTC\")")
	case err != nil:
		add("timezone %q is not a known IANA name: %v", c.Timezone, err)
	default:
		c.loc = loc
	}
	validDays := map[string]bool{"monday": true, "tuesday": true, "wednesday": true,
		"thursday": true, "friday": true, "saturday": true, "sunday": true}
	for _, d := range c.RunDays {
		if !validDays[strings.ToLower(d)] {
			add("run_days: %q is not a weekday name (use lowercase, e.g. \"monday\")", d)
		}
	}

	if len(probs) == 0 {
		return nil
	}
	return errors.New("config problems:\n  - " + strings.Join(probs, "\n  - "))
}

// CheckSecrets verifies the credentials needed by the stages about to run.
// Dry runs need none of them.
func (c *Config) CheckSecrets(needOpenAI, needYouTube, needAWS bool) error {
	var probs []string
	add := func(format string, args ...any) {
		probs = append(probs, fmt.Sprintf(format, args...))
	}

	if needOpenAI && !filled(c.OpenAI.APIKey) {
		add("openai.api_key is not filled in")
	}
	if needYouTube && c.YouTube.Enabled {
		if !filled(c.YouTube.ClientID) {
			add("youtube.client_id is not filled in (from your OAuth Desktop-app client in Google Cloud Console)")
		}
		if !filled(c.YouTube.ClientSecret) {
			add("youtube.client_secret is not filled in (from your OAuth Desktop-app client in Google Cloud Console)")
		}
		if !filled(c.YouTube.RefreshToken) {
			add("youtube.refresh_token is not filled in (run `autobotgo auth` on your local machine; it fills this in)")
		}
	}
	if needAWS && c.AWS.Enabled {
		if !filled(c.AWS.Bucket) {
			add("aws.bucket is not filled in")
		}
		if !filled(c.AWS.Region) {
			add("aws.region is not filled in")
		}
		if !filled(c.AWS.AccessKeyID) {
			add("aws.access_key_id is not filled in")
		}
		if !filled(c.AWS.SecretAccessKey) {
			add("aws.secret_access_key is not filled in")
		}
	}

	if len(probs) == 0 {
		return nil
	}
	return errors.New("config problems:\n  - " + strings.Join(probs, "\n  - "))
}

// CheckYouTubeClient verifies the two values `autobotgo auth` needs before
// it can run the browser flow (the flow itself produces the third).
func (c *Config) CheckYouTubeClient() error {
	var probs []string
	if !filled(c.YouTube.ClientID) {
		probs = append(probs, "youtube.client_id is not filled in")
	}
	if !filled(c.YouTube.ClientSecret) {
		probs = append(probs, "youtube.client_secret is not filled in")
	}
	if len(probs) == 0 {
		return nil
	}
	return errors.New("config problems (copy these from your OAuth Desktop-app client in Google Cloud Console):\n  - " +
		strings.Join(probs, "\n  - "))
}

// Save writes the config back to path (0600, since it holds secrets). Used by
// `autobotgo auth` to store the refresh token it obtained.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// WriteTemplate writes the default config as JSON to path. It refuses to
// overwrite an existing file.
func WriteTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists, not overwriting", path)
	}
	data, err := json.MarshalIndent(Default(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/thesimpledev/autobotgo/internal/run"
	"github.com/thesimpledev/autobotgo/internal/yt"
)

// Upload sends final.mp4 to YouTube (private by default) and records the
// video ID in youtube.json, which doubles as the stage's completion marker.
var Upload = run.Stage{
	Name:    NameUpload,
	Outputs: []run.PathFn{runFile(FileYouTube)},
	Timeout: 45 * time.Minute,
	Fn:      uploadFn,
}

// UploadResult is what lands in youtube.json.
type UploadResult struct {
	VideoID    string `json:"video_id"`
	URL        string `json:"url"`
	UploadedAt string `json:"uploaded_at"`
	Skipped    bool   `json:"skipped,omitempty"`
}

func uploadFn(ctx context.Context, r *run.Run) error {
	if !r.Cfg.YouTube.Enabled {
		r.Log.Info("youtube disabled in config, skipping upload")
		return writeJSON(r, FileYouTube, UploadResult{Skipped: true})
	}

	mdData, err := os.ReadFile(r.Path(FileMetadata))
	if err != nil {
		return err
	}
	var md VideoMetadata
	if err := json.Unmarshal(mdData, &md); err != nil {
		return fmt.Errorf("parsing %s: %w", FileMetadata, err)
	}

	ts := yt.TokenSource(ctx,
		r.Cfg.YouTube.ClientID,
		r.Cfg.YouTube.ClientSecret,
		r.Cfg.YouTube.RefreshToken)

	id, err := yt.Upload(ctx, r.Log, ts, yt.UploadParams{
		FilePath:      PathVideo(r),
		Title:         md.Title,
		Description:   md.Description,
		Tags:          md.Tags,
		CategoryID:    r.Cfg.YouTube.CategoryID,
		PrivacyStatus: r.Cfg.YouTube.PrivacyStatus,
		MadeForKids:   r.Cfg.YouTube.MadeForKids,
		PlaylistID:    r.Cfg.YouTube.PlaylistID,
	})
	if err != nil {
		return err
	}
	return writeJSON(r, FileYouTube, UploadResult{
		VideoID:    id,
		URL:        "https://youtu.be/" + id,
		UploadedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(r *run.Run, rel string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return r.WriteFile(rel, append(data, '\n'))
}

package yt

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	youtube "google.golang.org/api/youtube/v3"
)

// UploadParams describes one video upload.
type UploadParams struct {
	FilePath      string
	Title         string
	Description   string
	Tags          []string
	CategoryID    string
	PrivacyStatus string
	MadeForKids   bool
	PlaylistID    string
}

// Upload sends the video with resumable upload and returns the video ID.
func Upload(ctx context.Context, log *slog.Logger, ts oauth2.TokenSource, p UploadParams) (string, error) {
	svc, err := youtube.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", fmt.Errorf("creating youtube client: %w", err)
	}

	f, err := os.Open(p.FilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	video := &youtube.Video{
		Snippet: &youtube.VideoSnippet{
			Title:       p.Title,
			Description: p.Description,
			Tags:        p.Tags,
			CategoryId:  p.CategoryID,
		},
		Status: &youtube.VideoStatus{
			PrivacyStatus:           p.PrivacyStatus,
			SelfDeclaredMadeForKids: p.MadeForKids,
			// The video is fully AI-generated; declare it.
			ContainsSyntheticMedia: true,
		},
	}
	// SelfDeclaredMadeForKids=false and ContainsSyntheticMedia must still
	// be serialized when false/true respectively.
	video.Status.ForceSendFields = append(video.Status.ForceSendFields,
		"SelfDeclaredMadeForKids", "ContainsSyntheticMedia")

	log.Info("uploading to YouTube", "title", p.Title, "privacy", p.PrivacyStatus)
	call := svc.Videos.Insert([]string{"snippet", "status"}, video)
	resp, err := call.Media(f, googleapi.ChunkSize(8*1024*1024)).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("youtube upload: %w", err)
	}
	log.Info("upload complete", "video_id", resp.Id, "url", "https://youtu.be/"+resp.Id)

	if p.PlaylistID != "" {
		_, err = svc.PlaylistItems.Insert([]string{"snippet"}, &youtube.PlaylistItem{
			Snippet: &youtube.PlaylistItemSnippet{
				PlaylistId: p.PlaylistID,
				ResourceId: &youtube.ResourceId{Kind: "youtube#video", VideoId: resp.Id},
			},
		}).Context(ctx).Do()
		if err != nil {
			// The video is up; a playlist failure shouldn't fail the run.
			log.Warn("adding to playlist failed", "playlist", p.PlaylistID, "error", err)
		}
	}
	return resp.Id, nil
}

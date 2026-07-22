package stage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/thesimpledev/autobotgo/internal/run"
)

// Archive (optional, aws.enabled) tars the run's artifacts and uploads
// the archive to S3. It runs before cleanup so the media still exists.
var Archive = run.Stage{
	Name:    NameArchive,
	Outputs: []run.PathFn{runFile(FileArchive)},
	Timeout: 45 * time.Minute,
	Fn:      archiveFn,
}

// ArchiveResult is what lands in archive.json.
type ArchiveResult struct {
	S3Key      string `json:"s3_key"`
	SizeBytes  int64  `json:"size_bytes"`
	UploadedAt string `json:"uploaded_at"`
	Skipped    bool   `json:"skipped,omitempty"`
}

func archiveFn(ctx context.Context, r *run.Run) error {
	if !r.Cfg.AWS.Enabled {
		r.Log.Info("aws disabled in config, skipping archive")
		return writeJSON(r, FileArchive, ArchiveResult{Skipped: true})
	}

	tarPath := r.Path(r.Date + ".tar.gz")
	if err := buildTarball(r, tarPath); err != nil {
		return err
	}
	defer os.Remove(tarPath) // uploaded copy is the archive; don't keep it locally

	fi, err := os.Stat(tarPath)
	if err != nil {
		return err
	}
	key := archiveKey(r.Cfg.AWS.Prefix, r.Date)
	if err := uploadS3(ctx, r, tarPath, key); err != nil {
		return err
	}
	r.Log.Info("archive uploaded", "bucket", r.Cfg.AWS.Bucket, "key", key, "bytes", fi.Size())

	return writeJSON(r, FileArchive, ArchiveResult{
		S3Key:      key,
		SizeBytes:  fi.Size(),
		UploadedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// archiveKey builds the S3 object key. An empty prefix must not produce a
// leading slash, which would create an unnamed top-level folder in the
// bucket.
func archiveKey(prefix, date string) string {
	name := date + ".tar.gz"
	if p := strings.Trim(prefix, "/"); p != "" {
		return p + "/" + name
	}
	return name
}

// archiveEntry is one file in the tarball: where it is on disk, and the
// name it gets inside the archive.
type archiveEntry struct {
	path string
	name string
}

// archiveEntries lists what goes in the tarball: the story, the media
// (still present, cleanup runs after), and the bookkeeping files.
func archiveEntries(r *run.Run) []archiveEntry {
	entries := []archiveEntry{
		{PathStory(r), "story.md"},
		{PathNarration(r), "narration.wav"},
		{PathVideo(r), "final.mp4"},
		{PathWorld(r), "world.md"},
	}
	if imgs, err := listImages(DirImages(r)); err == nil {
		for _, p := range imgs {
			entries = append(entries, archiveEntry{p, filepath.Join("images", filepath.Base(p))})
		}
	}
	for _, name := range []string{FileImagePrompts, FileImagesDone, FileTimeline, FileFiltergraph, FileMetadata, FileYouTube, FileWorld, "run.log"} {
		entries = append(entries, archiveEntry{r.Path(name), name})
	}
	return entries
}

func buildTarball(r *run.Run, tarPath string) error {
	tmp := tarPath + ".tmp"
	f, err := os.Create(tmp) // #nosec G304 -- path is inside the run directory
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	for _, e := range archiveEntries(r) {
		fi, err := os.Stat(e.path)
		if os.IsNotExist(err) {
			r.Log.Warn("archive: artifact missing, skipping", "file", e.name)
			continue
		}
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = e.name
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		src, err := os.Open(e.path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(tw, src); err != nil {
			_ = src.Close()
			return err
		}
		_ = src.Close()
	}

	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, tarPath)
}

func uploadS3(ctx context.Context, r *run.Run, path, key string) error {
	cfgAWS := r.Cfg.AWS
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfgAWS.Region),
	}
	if cfgAWS.AccessKeyID != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfgAWS.AccessKeyID, cfgAWS.SecretAccessKey, "")))
	}
	var s3Opts []func(*s3.Options)
	if cfgAWS.Endpoint != "" {
		// S3-compatible stores generally reject or mishandle the
		// integrity checksums the SDK now sends by default, so ask for
		// them only where the protocol requires them. Real AWS keeps the
		// default behavior.
		opts = append(opts, awsconfig.WithRequestChecksumCalculation(
			aws.RequestChecksumCalculationWhenRequired))
		endpoint := cfgAWS.Endpoint
		s3Opts = append(s3Opts, func(o *s3.Options) { o.BaseEndpoint = &endpoint })
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("aws config: %w", err)
	}
	f, err := os.Open(path) // #nosec G304 -- path is the tarball this run just built
	if err != nil {
		return err
	}
	defer f.Close()

	uploader := manager.NewUploader(s3.NewFromConfig(awsCfg, s3Opts...))
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: &r.Cfg.AWS.Bucket,
		Key:    &key,
		Body:   f,
	})
	if err != nil {
		return fmt.Errorf("s3 upload: %w", err)
	}
	return nil
}

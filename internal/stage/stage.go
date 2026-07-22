// Package stage implements the pipeline stages: story, world, tts,
// images, video, metadata, upload, archive, cleanup.
//
// Artifact layout under the config's base directory:
//
//	stories/<date>.md        permanent, the accumulating story archive
//	audio/<date>.wav         transient, emptied by the cleanup stage
//	audio/<date>-chunks/     transient TTS intermediates
//	images/<date>/img_NN.png transient
//	videos/<date>.mp4        transient (it lives on YouTube after upload)
//	videos/<date>-segments/  transient render pieces
//	world.md                 permanent, the capped world bible
//	runs/<date>/             small bookkeeping JSON + logs, kept
package stage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thesimpledev/autobotgo/internal/run"
)

// Stage names. These are the identifiers the --stage flag accepts, the
// keys the dry-run pipeline swaps on, and what run.StageError reports, so
// they are declared once here rather than spelled out at each use.
const (
	NameStory    = "story"
	NameWorld    = "world"
	NameTTS      = "tts"
	NameImages   = "images"
	NameVideo    = "video"
	NameMetadata = "metadata"
	NameUpload   = "upload"
	NameArchive  = "archive"
	NameCleanup  = "cleanup"
)

// Bookkeeping files inside the run dir.
const (
	FileImagePrompts = "image_prompts.json"
	FileImagesDone   = "images.json" // written last; the image stage's completion marker
	FileTimeline     = "timeline.json"
	FileFiltergraph  = "filtergraph.txt"
	FileMetadata     = "metadata.json"
	FileYouTube      = "youtube.json"
	FileWorld        = "world.json"   // world stage completion marker
	FileArchive      = "archive.json" // archive stage completion marker
	FileCleanup      = "cleanup.json" // cleanup stage completion marker; marks the day done
)

// WorldFile is the world bible's name in the base directory.
const WorldFile = "world.md"

// Typed artifact locations.

func PathStory(r *run.Run) string {
	return filepath.Join(r.Base, "stories", r.Date+".md")
}

func PathNarration(r *run.Run) string {
	return filepath.Join(r.Base, "audio", r.Date+".wav")
}

func DirTTSChunks(r *run.Run) string {
	return filepath.Join(r.Base, "audio", r.Date+"-chunks")
}

func DirImages(r *run.Run) string {
	return filepath.Join(r.Base, "images", r.Date)
}

func PathVideo(r *run.Run) string {
	return filepath.Join(r.Base, "videos", r.Date+".mp4")
}

func DirSegments(r *run.Run) string {
	return filepath.Join(r.Base, "videos", r.Date+"-segments")
}

func PathWorld(r *run.Run) string {
	return filepath.Join(r.Base, WorldFile)
}

// runFile declares a bookkeeping file as a stage output.
func runFile(name string) run.PathFn {
	return func(r *run.Run) string { return r.Path(name) }
}

// VideoMetadata is what the metadata stage produces and the upload stage
// consumes.
type VideoMetadata struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

func ensureDirOf(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o750)
}

func renameFile(from, to string) error {
	return os.Rename(from, to)
}

// writeArtifact writes data to an absolute artifact path atomically.
func writeArtifact(path string, data []byte) error {
	return run.WriteFileAtomic(path, data)
}

// listImages returns the absolute paths of img_*.png in the given
// directory, in order. Shared by the image, video, and archive stages.
func listImages(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, "img_") && strings.HasSuffix(name, ".png") {
			paths = append(paths, filepath.Join(dir, name))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no img_*.png files in %s", dir)
	}
	sort.Strings(paths)
	return paths, nil
}

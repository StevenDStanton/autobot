package stage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thesimpledev/autobotgo/internal/ffmpeg"
	"github.com/thesimpledev/autobotgo/internal/openai"
	"github.com/thesimpledev/autobotgo/internal/run"
)

// TTS turns the story into audio/<date>.wav. The story is chunked (the
// API caps input at 4096 chars), each chunk synthesized to WAV, and the
// segments joined losslessly with the ffmpeg concat demuxer. WAV, not
// MP3: MP3 encoder padding puts audible clicks at segment seams.
var TTS = run.Stage{
	Name:    NameTTS,
	Outputs: []run.PathFn{PathNarration},
	Timeout: 15 * time.Minute,
	Fn:      ttsFn,
}

const (
	// charsPerToken converts the config's token-based chunk limit into the
	// character limit the speech API actually enforces. English prose runs
	// closer to 4, so 3.5 deliberately under-estimates: a chunk that comes
	// out slightly short is free, one that comes out long is a failed call.
	charsPerToken = 3.5
	// speechCharHeadroom keeps the chunk clear of the hard cap, leaving
	// room for the sentence-boundary split to overshoot slightly.
	speechCharHeadroom = 100
)

func ttsFn(ctx context.Context, r *run.Run) error {
	text, err := os.ReadFile(PathStory(r))
	if err != nil {
		return err
	}

	// The config expresses the limit in tokens, but the API's limit is in
	// characters, so convert and then clamp under the hard cap.
	maxChars := min(int(float64(r.Cfg.OpenAI.TTSMaxChunkTok)*charsPerToken), openai.SpeechMaxChars-speechCharHeadroom)
	chunks, err := ChunkText(string(text), maxChars)
	if err != nil {
		return err
	}
	r.Log.Info("synthesizing narration", "chunks", len(chunks), "voice", r.Cfg.OpenAI.TTSVoice)

	chunkDir := DirTTSChunks(r)
	if err := os.MkdirAll(chunkDir, 0o750); err != nil {
		return err
	}
	client := openai.New(&r.Cfg.OpenAI, r.Log)
	var listing strings.Builder
	for i, chunk := range chunks {
		wavPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%03d.wav", i))
		txtPath := filepath.Join(chunkDir, fmt.Sprintf("chunk_%03d.txt", i))
		if err := writeArtifact(txtPath, []byte(chunk)); err != nil {
			return err
		}
		// Chunks that already exist are kept (resume within the stage:
		// a failure at chunk 7 doesn't re-buy chunks 0-6).
		if fi, err := os.Stat(wavPath); err != nil || fi.Size() == 0 {
			audio, err := client.Speech(ctx, chunk)
			if err != nil {
				return fmt.Errorf("chunk %d/%d: %w", i+1, len(chunks), err)
			}
			if err := writeArtifact(wavPath, audio); err != nil {
				return err
			}
			r.Log.Info("chunk synthesized", "chunk", i+1, "of", len(chunks), "chars", len(chunk))
		} else {
			r.Log.Info("chunk exists, keeping", "chunk", i+1, "of", len(chunks))
		}
		// concat demuxer paths are relative to the listing file's dir.
		fmt.Fprintf(&listing, "file 'chunk_%03d.wav'\n", i)
	}

	concatPath := filepath.Join(chunkDir, "concat.txt")
	if err := writeArtifact(concatPath, []byte(listing.String())); err != nil {
		return err
	}
	out := PathNarration(r)
	if err := ensureDirOf(out); err != nil {
		return err
	}
	tmp := out + ".tmp.wav"
	if err := ffmpeg.Run(ctx, r.Log,
		"-f", "concat", "-safe", "0",
		"-i", concatPath,
		"-c", "copy", tmp,
	); err != nil {
		return err
	}
	return renameFile(tmp, out)
}

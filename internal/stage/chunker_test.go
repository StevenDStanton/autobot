package stage

import (
	"strings"
	"testing"
)

func TestChunkTextShortStaysWhole(t *testing.T) {
	text := "Para one.\n\nPara two."
	chunks, err := ChunkText(text, 4000)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 1 || chunks[0] != text {
		t.Fatalf("got %q", chunks)
	}
}

func TestChunkTextRespectsLimit(t *testing.T) {
	var paras []string
	for range 40 {
		paras = append(paras, strings.Repeat("All work and no play makes for a dull story. ", 8))
	}
	chunks, err := ChunkText(strings.Join(paras, "\n\n"), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) < 2 {
		t.Fatal("expected multiple chunks")
	}
	var total int
	for i, c := range chunks {
		if len(c) > 1000 {
			t.Errorf("chunk %d is %d chars", i, len(c))
		}
		total += len(strings.Fields(c))
	}
	want := 0
	for _, p := range paras {
		want += len(strings.Fields(p))
	}
	if total != want {
		t.Errorf("words lost in chunking: got %d, want %d", total, want)
	}
}

func TestChunkTextSplitsOversizedParagraphBySentence(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("This sentence is repeated to build one huge paragraph. ", 30))
	chunks, err := ChunkText(long, 500)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range chunks {
		if len(c) > 500 {
			t.Errorf("chunk %d is %d chars", i, len(c))
		}
		if !strings.HasSuffix(strings.TrimSpace(c), ".") {
			t.Errorf("chunk %d does not end at a sentence boundary: %q", i, c[len(c)-30:])
		}
	}
}

func TestChunkTextGiantSentenceErrors(t *testing.T) {
	if _, err := ChunkText(strings.Repeat("word ", 300), 500); err == nil {
		t.Fatal("expected error for an unbreakable sentence")
	}
}

func TestChunkTextEmpty(t *testing.T) {
	if _, err := ChunkText("  \n\n  ", 500); err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestSplitSentencesQuotes(t *testing.T) {
	got := splitSentences(`"Stop!" she said. He did not stop.`)
	if len(got) != 3 {
		t.Fatalf("got %d sentences: %q", len(got), got)
	}
}

package stage

import (
	"fmt"
	"strings"
)

// ChunkText splits story text into pieces no longer than maxChars, only
// breaking at paragraph boundaries, or sentence boundaries when a single
// paragraph is too long. TTS prosody glitches mid-sentence, so we never
// split inside one; a single sentence longer than maxChars is an error
// (at 4096 chars that would be pathological input).
func ChunkText(text string, maxChars int) ([]string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("no text to chunk")
	}

	var units []string // paragraphs, or sentences of oversized paragraphs
	for para := range strings.SplitSeq(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if len(para) <= maxChars {
			units = append(units, para)
			continue
		}
		for _, s := range splitSentences(para) {
			if len(s) > maxChars {
				return nil, fmt.Errorf("sentence of %d chars exceeds chunk limit %d (begins %.80q)", len(s), maxChars, s)
			}
			units = append(units, s)
		}
	}

	// Greedily pack units, joining with blank lines so paragraph pauses
	// survive into the synthesized audio.
	var chunks []string
	var cur strings.Builder
	for _, u := range units {
		sep := 0
		if cur.Len() > 0 {
			sep = 2
		}
		if cur.Len()+sep+len(u) > maxChars {
			chunks = append(chunks, cur.String())
			cur.Reset()
			sep = 0
		}
		if sep > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(u)
	}
	if cur.Len() > 0 {
		chunks = append(chunks, cur.String())
	}
	return chunks, nil
}

// splitSentences breaks prose on `. `, `! `, `? ` (and their quote-trailing
// forms). Good enough for LLM-generated prose; not a general tokenizer.
func splitSentences(s string) []string {
	var out []string
	start := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '.', '!', '?':
			j := i + 1
			// Include trailing closing quotes with the sentence.
			for j < len(runes) && (runes[j] == '"' || runes[j] == '\'' || runes[j] == '”' || runes[j] == '’') {
				j++
			}
			if j >= len(runes) || runes[j] == ' ' {
				sent := strings.TrimSpace(string(runes[start:j]))
				if sent != "" {
					out = append(out, sent)
				}
				start = j
				i = j
			}
		}
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		out = append(out, tail)
	}
	return out
}

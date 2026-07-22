package stage

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/thesimpledev/autobotgo/internal/openai"
	"github.com/thesimpledev/autobotgo/internal/run"
)

// Story writes the narration script to stories/<date>.md, guided by the
// user's story.md brief, the world.md canon, and, when RAG is enabled
// and the vector store exists, retrieval over the full story archive.
//
// A single generation pass reliably drops a few of the brief's many
// rules, so the stage runs a draft -> audit -> revise loop: a strict
// editor pass lists every rule violation and logic hole, and the story
// model rewrites until the audit passes (bounded rounds).
var Story = run.Stage{
	Name:    NameStory,
	Outputs: []run.PathFn{PathStory},
	Timeout: 30 * time.Minute, // reasoning-tier story models are slow
	Fn:      storyFn,
}

// maxAuditRounds bounds the audit/revise loop; the final draft is used
// even if the last audit still lists issues.
const maxAuditRounds = 2

const storySystemPrompt = `You are a storyteller writing scripts for narrated YouTube videos.
All stories take place in one shared, continuing world. Honor the WORLD BIBLE if one is
provided, but understand what continuity means here: the Institute, its procedures, and
established facts persist; SCENARIOS never repeat. Each broadcast is a new incident, in a
new setting, with a phenomenon this series has not covered. You will be given today's
premise. Follow it exactly.
If a story archive is searchable, consult it to stay consistent with the past, not to
imitate it. Each story must stand alone for a first-time viewer.
Write a complete, self-contained story following the brief exactly. Every rule in it exists
because its absence broke an earlier story.
Output ONLY the story text itself: no title, no headings, no scene directions, no markdown.
Separate paragraphs with blank lines. The text will be read aloud exactly as written,
so avoid abbreviations, symbols, and anything that reads poorly as speech.`

// premiseSystem forces divergence: eight premises, each in a different
// setting category, none resembling past broadcasts. The model never
// picks the winner, code rolls the dice, so it cannot play favorites.
const premiseSystem = `You invent premises for an anthology of unsettling public-safety broadcasts
set in one shared world (brief and canon provided). Produce exactly 8 one-sentence premises for
the next broadcast.
Rules:
- Each premise names WHERE it happens, WHO it happens to (an ordinary person and their routine),
  and WHAT anomalous phenomenon occurs.
- The 8 premises use these setting categories, in order:
  1 deep woods or camping, 2 city street or large building, 3 underground (sewer, tunnel,
  basement, cave), 4 water (lake, river, coast, pool), 5 workplace or night shift,
  6 road, transit, or travel, 7 rural (farm, field, small town), 8 home or neighborhood.
- Vary the phenomena: no two premises may share an entity type or mechanism, and none may
  reuse the setting or phenomenon of any broadcast recorded in the world bible.
- Every premise must fit the series rules: non-violent, non-sexual, real-world technology,
  tellable through plausible sources, workable as a public advisory.
Return ONLY JSON: {"premises": ["...", "...", "...", "...", "...", "...", "...", "..."]}`

const storyAuditSystem = `You are a ruthless continuity and logic editor for a story series.
You receive the series brief (the rules), today's assigned premise, and a draft story.
Audit the draft strictly:
- check every rule in the brief, one by one
- check the story follows the assigned premise: its setting and phenomenon, not a different one
- check the story does not reuse the setting or phenomenon of any broadcast in the world bible
- check internal logic: timeline arithmetic, physical and spatial paths, cause and effect
- check that every specific detail traces to a stated, plausible, real-world source
- check that exactly one subtle tell or wrongness carries the story and nothing is over-explained
- check that it contradicts nothing in the world bible
List every violation, quoting the offending text. Report only genuine problems.
Return ONLY JSON: {"pass": true|false, "issues": ["...", "..."]}
pass is true only when there are no issues.`

const storyReviseSystem = `You are revising a draft story for a narrated video series.
You receive the series brief, the draft, and an editor's list of violations. Rewrite the
story to fix every listed issue while keeping everything that works: same incident, same
structure, same voice, unless an issue demands otherwise. Obey the brief exactly.
Output ONLY the corrected story text: no title, no headings, no markdown, no commentary.`

type auditResult struct {
	Pass   bool     `json:"pass"`
	Issues []string `json:"issues"`
}

type premiseList struct {
	Premises []string `json:"premises"`
}

// pickPremise asks the utility model for eight setting-spread premises,
// then rolls the dice. The full candidate list and the roll are saved to
// the run dir for review.
func pickPremise(ctx context.Context, r *run.Run, client *openai.Client, briefCtx string) (string, error) {
	reply, err := client.Text(ctx, premiseSystem, briefCtx)
	if err != nil {
		return "", fmt.Errorf("premises: %w", err)
	}
	jsonPart, err := openai.ExtractJSON(reply)
	if err != nil {
		return "", fmt.Errorf("premises: %w", err)
	}
	var list premiseList
	if err := json.Unmarshal([]byte(jsonPart), &list); err != nil {
		return "", fmt.Errorf("premises: parsing: %w", err)
	}
	if len(list.Premises) < 4 {
		return "", fmt.Errorf("premises: got only %d candidates", len(list.Premises))
	}
	// #nosec G404 -- picking which story idea to write is not a security
	// decision; it only needs to be unbiased, not unpredictable.
	pick := rand.IntN(len(list.Premises))
	premise := list.Premises[pick]
	r.Log.Info("premise chosen by dice roll", "pick", pick+1, "of", len(list.Premises), "premise", premise)

	record, err := json.MarshalIndent(map[string]any{"premises": list.Premises, "picked": pick}, "", "  ")
	if err == nil {
		if werr := r.WriteFile("premises.json", record); werr != nil {
			r.Log.Warn("could not save premises record", "error", werr)
		}
	}
	return premise, nil
}

func storyFn(ctx context.Context, r *run.Run) error {
	brief, err := os.ReadFile(r.Cfg.Resolve(r.Cfg.Story.PromptFile))
	if err != nil {
		return fmt.Errorf("reading story prompt file: %w", err)
	}

	var briefCtx strings.Builder
	briefCtx.WriteString(strings.TrimSpace(string(brief)))
	if world, err := os.ReadFile(PathWorld(r)); err == nil && len(world) > 0 {
		briefCtx.WriteString("\n\n--- WORLD BIBLE (established canon; stay consistent with it) ---\n\n")
		briefCtx.Write(world)
	}
	target := fmt.Sprintf(
		"Target length: %d to %d words (about %d minutes read aloud at a calm pace).",
		r.Cfg.Story.TargetWordsMin, r.Cfg.Story.TargetWordsMax, r.Cfg.Story.TargetMinutes)

	client := openai.New(&r.Cfg.OpenAI, r.Log)
	storyModel := r.Cfg.OpenAI.StoryModelName()
	vsID := ""
	if r.Cfg.World.RAGEnabled {
		vsID = r.Cfg.World.VectorStoreID
	}

	// Premises: the model diverges, the dice decide. Eight candidates in
	// eight forced setting categories; code picks one at random so the
	// model's favorite scenario never gets a vote.
	premise, err := pickPremise(ctx, r, client, briefCtx.String())
	if err != nil {
		return err
	}

	// Draft.
	r.Log.Info("writing story draft", "model", storyModel, "archive_search", vsID != "")
	draft, err := client.Generate(ctx, storyModel, storySystemPrompt,
		fmt.Sprintf("%s\n\n--- TODAY'S PREMISE (write exactly this incident) ---\n\n%s\n\n---\nWrite one new story now. %s",
			briefCtx.String(), premise, target), vsID)
	if err != nil {
		return err
	}

	// Audit / revise loop.
	for round := 1; round <= maxAuditRounds; round++ {
		auditInput := fmt.Sprintf("SERIES BRIEF AND CANON:\n\n%s\n\n--- ASSIGNED PREMISE ---\n\n%s\n\n--- DRAFT STORY ---\n\n%s",
			briefCtx.String(), premise, draft)
		reply, err := client.Text(ctx, storyAuditSystem, auditInput)
		if err != nil {
			return fmt.Errorf("audit round %d: %w", round, err)
		}
		jsonPart, err := openai.ExtractJSON(reply)
		if err != nil {
			return fmt.Errorf("audit round %d: %w", round, err)
		}
		var audit auditResult
		if err := json.Unmarshal([]byte(jsonPart), &audit); err != nil {
			return fmt.Errorf("audit round %d: parsing verdict: %w", round, err)
		}
		if audit.Pass || len(audit.Issues) == 0 {
			r.Log.Info("story audit passed", "round", round)
			break
		}
		r.Log.Info("story audit found issues, revising",
			"round", round, "issues", len(audit.Issues))
		for _, issue := range audit.Issues {
			r.Log.Info("audit issue", "round", round, "issue", issue)
		}
		if round == maxAuditRounds {
			r.Log.Warn("audit issues remain after final round; using latest draft")
			break
		}
		reviseInput := fmt.Sprintf(
			"SERIES BRIEF AND CANON:\n\n%s\n\n--- ASSIGNED PREMISE ---\n\n%s\n\n--- DRAFT STORY ---\n\n%s\n\n--- EDITOR'S ISSUES ---\n- %s\n\n%s",
			briefCtx.String(), premise, draft, strings.Join(audit.Issues, "\n- "), target)
		draft, err = client.Generate(ctx, storyModel, storyReviseSystem, reviseInput, vsID)
		if err != nil {
			return fmt.Errorf("revision round %d: %w", round, err)
		}
	}

	words := len(strings.Fields(draft))
	r.Log.Info("story finished", "words", words)
	if words < r.Cfg.Story.TargetWordsMin/2 {
		return fmt.Errorf("story came back suspiciously short (%d words); not proceeding", words)
	}
	return writeArtifact(PathStory(r), []byte(draft))
}

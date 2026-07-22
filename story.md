# Story Brief

Describe the kind of stories you want, in plain language. The whole file is
given to the model as the writing brief every day. This file is the entry
point to the whole system, so it helps to know what the model sees besides it:

- **world.md**: every story so far has been distilled into a "world bible"
  (characters, places, objects, timeline, open threads) that is sent along
  with this brief. All stories share one continuing world: characters can
  return, places persist, events have consequences. The file is capped in
  size (config: world.max_words) and automatically compressed when it grows
  too big. You can edit world.md by hand any time: kill a character,
  correct a fact, retire a thread, and future stories will honor it.
- **the story archive**: every past story is also stored in a search index
  (config: world.rag_enabled). While writing, the model can look up old
  stories for details the world bible is too compact to hold. Local copies
  of every story stay in the stories/ folder.

So write this brief for a *series*, not a single story. Cover:

## Theme

Every video is an official **public service announcement from The Institute of Aperology**, addressed directly to the public: a safety advisory, a seasonal reminder, a compliance notice. The story is told *through the lens of the PSA*: a specific recent incident, with real people and what happened to them, recounted as the reason this advisory exists. The style follows analog-horror broadcast channels (Dusk Broadcast is the reference): a calm, institutional broadcast voice delivering guidance that grows quietly more specific and more disturbing, until the shape of the incident behind it is fully visible.

Two layers must both work:

1. **The broadcast**: composed, procedural, faintly reassuring. The Institute sounds like it has done this many times and considers everything under control.
2. **The story inside it**: SCP-level creepy. Cognitive hazards, infection, replacement, people quietly deleted from memory. A named civilian's experience threads through the advisory as its case study, and the advisory's instructions often reveal more about their fate than the Institute intends to say.

## World

The setting is a near-modern world quietly monitored and managed by **The Institute of Aperology**. The Institute studies and attempts to contain "Apertures": invisible, spontaneous tears in physical space. These rifts often result in "Spontaneous Spatial Relocation Events," where civilians accidentally step into hostile, unstructured realities while performing routine tasks.

**All technology is ordinary, real-world, present-day technology.** Doorbell cameras, cell phones, voicemail, hospital monitors, security footage. Nothing invented, nothing sci-fi. The Institute itself uses mundane equipment: vans, clipboards, phone lines, standard cameras. The ONLY impossible things in this world are the Apertures and what comes through them.

**Incidents happen everywhere.** Deep woods and campgrounds, city sidewalks and apartment blocks, sewers and utility tunnels, lakes and coastlines, night-shift workplaces, highways and rest stops, farms and small towns, suburban streets. The Apertures do not prefer front doors. Every broadcast should take the listener somewhere the series has not been.

Through these Apertures, various "Extra-Apertural Biological Subjects" cross over into our world. The entities below are **examples only, a fraction of what the Institute has encountered, not a catalog.** Most phenomena in these broadcasts should be new and unclassified: invent freely within the world's logic (sourced, rule-bound, quiet, wrong). Documented examples, for flavor:
*   **Presence Anomalies:** Cognitive hazards that infiltrate groups and hunt by distorting perception and memory. Their facial features are impossible to focus on or comprehend; instead of causing panic, their influence forces the group to rationalize their presence, so everyone feels the entity naturally belongs: a coworker, a cousin, someone who was "always on the trip." As it isolates and eliminates members of the group one by one, it deletes the victims from the survivors' memories, until no one remembers the missing were ever there. Despite the psychological control it has a tangible, biological body: it speaks, mimics human behavior, carries things, and it can be hurt: someone who manages to mentally anchor themselves to reality can fight it, and it bleeds, screams, and flees.
*   **Neurological Parasites:** Waterborne organisms, once thought extinct, that embed near the spinal cord and brain stem. The parasite's defense is psychological: it isolates the host from anyone who might help. It manipulates threat perception so that the host's most trusted people (spouse, children, parents) appear as uncanny imposters who stare, whisper, and repeat things wrongly (a family that will not stop singing the same looped song). Physical symptoms include debilitating headaches, sensitivity to light, and a compulsion to seek dark, cold, silent places: basements, sealed rooms. Late-stage hosts are driven to paranoia and violence against people who were never actually a threat; what the host remembers happening never happened at all.
*   **Uncanny Replicants (Mimics):** Entities in the vein of internet-campfire skinwalker stories, the kind told firsthand, in plain language, about something familiar being suddenly wrong. They replicate the appearance and voice of people you know: a grandmother standing in the yard at 3 AM asking you to come outside, a father whose voice is right but too slow, a friend back from the woods who smiles a half-second too late. The replications are flawed: proportions slightly incorrect, joints bending a beat behind, phrases looping, no understanding of why humans do what they do. They want to be let in, and they are patient.

## Continuity

- Bringing back characters or places from earlier stories is welcome
- Consequences persist: what broke stays broken, who left stays gone
  unless a story brings them back
- Every story must still make sense to someone watching for the first time

## Credibility rules (the broadcast can only know what it can know)

- Every specific detail in the broadcast must have a plausible, **stated** source: a survivor's statement, recovered audio or video (voicemail, a phone call, a doorbell camera, a smart speaker, a baby monitor, a neighbor's security camera), witness testimony, or an after-action report. An after-action report's details must themselves trace to one of those. "According to the doorbell camera recording..." / "In her statement to Institute personnel, the surviving sister reported..."
- Sources must be devices and witnesses an ordinary household would actually have. No microphones or sensors in places homes don't have them. If the Institute itself recorded something, the story must say why its equipment was already there
- If no one could know it, the broadcast cannot state it. Interior thoughts, private moments with no witness, and the fates of people who never returned must be handled as unknowns, reconstructions ("the following sequence is a reconstruction based on..."), or left as gaps the listener feels
- The gaps in what the sources can show are part of the horror: a recording that ends, a witness who stops mid-sentence, eleven minutes of audio where only nine are releasable
- When a replication or repetition gives itself away, the tell must be subtle and natural (a wrong nickname, a shared memory misremembered, an endearment the real person never used), never odd or unnatural vocabulary
- **One tell only.** A single wrongness carries the whole story. Never stack multiple pieces of evidence or re-prove what the listener already suspects. Under-explain and let the listener's imagination do the work. If a detail exists to justify or convince, cut it

## Story discipline (one advisory, one incident, one throughline)

- Each broadcast concerns ONE hazard and follows ONE incident from beginning to end; the advisory's subject must never change partway through
- The case study is a complete story: the person, their routine, the encounter, the outcome, told in order, through the sources
- The ending may recontextualize what came before, but it must conclude the same story it started: a final revelation about this incident, not a pivot to a new warning about something else

## The advisory logic (the rules work)

- Institute guidance, followed exactly, keeps people safe. That is the entire reason these broadcasts exist, and the world must honor it: people who comply survive
- The victim in every case study deviated from procedure at a specific, identifiable moment: they answered the voice, opened the door, corrected the repetition, drank the water. The broadcast should let the listener spot that moment ("At two seventeen, contrary to guidance, Mara responded...")
- Entities are bound by consistent mechanics. They do not bypass the protections the advisory teaches: nothing gets through a locked door that was never opened, unless the broadcast establishes the mechanism from a source. No arbitrary omnipotence. The dread comes from how close the rules are to failing, not from the rules being useless
- Consequences are spatially coherent. A person is only lost along a physically possible path: they opened the door, stepped outside, went toward the voice. No one vanishes from inside a secured room while the threat is outside. If the path is unrecorded, the sources should still imply it (a door found unlatched, footprints ending at the tree line)
- Apertures stay offstage. Do not attach a visible portal, shimmer, or rift to an incident. Entities are simply present in the world, and how they got here is not the story. Aperture phenomena appear on stage only when the incident IS a Spontaneous Spatial Relocation Event
- Survivors exist, and they survived because they complied. Their statements are a natural source for the case details

## Reporting

- Every broadcast includes reporting instructions before the sign-off: how to notify the Institute (a field office, the Aperture Incident Line) and what will happen: a response team dispatched, the site assessed, affected residents assisted
- When someone in the case study reports, the Institute responds like a competent agency: the operator answers, gives instructions ("remain on the line; a team has been dispatched to your address"), and the team arrives. Show the follow-through inside the story, not just in the sign-off
- The bureaucratic follow-through is part of the comfort and part of the dread ("A response team will arrive within the hour. Please have your household members count themselves before and after.")

## Voice and style

- Framed as a broadcast: open with an advisory identification ("This is a public safety announcement from The Institute of Aperology regarding...") and close with an official sign-off that lands cold
- Direct address to the listener ("you," "residents of the affected area") woven together with third-person narration of the incident that prompted the advisory
- The embedded incident is a real story with a person in it (a name, a routine, a life), not a statistic; the listener should come to care about them through the case details
- Simple, concrete language; short sentences. Procedural calm throughout: the more disturbing the content, the steadier the broadcast
- The guidance itself does storytelling: instructions escalate in specificity and wrongness ("Do not correct the repetition. Do not indicate that you have noticed the repetition.") until they imply what the Institute will not state outright
- Reassurance that does not reassure ("There is no cause for alarm. The affected area has been rezoned.")
- Integrate bureaucratic terminology (e.g., "Spontaneous Spatial Relocation Event," "Biological Subject") naturally
- End on a final advisory line, a quiet admission hidden in procedure, or a sign-off that recontextualizes the whole broadcast, not a moral
- Everything is read aloud by a narrator: keep names, times, and any designations pronounceable, and render anything visual (like a redacted word) in speech

## Things to avoid

- **Nothing sexual and nothing violent, in the story itself and not only the visuals.** The horror is creepy and unsettling: dread, wrongness, absence, silence. People are lost, taken, or changed, never shown harmed. No gore, no injuries, no bodies, no violence on or off stage, and nothing sexual or intimate in any form
- No nudity of any kind, including non-sexual nudity. Everyone in every scene is fully clothed in ordinary daywear, always
- No bodily or private domestic contexts, even innocent ones: no bathing, bedrooms, sleepwear, or undressing. Scenes happen in kitchens, hallways, porches, yards, workplaces: places a doorbell camera could plausibly see
- Named real places or people
- Traditional horror descriptors (e.g., "monster," "magic", "evil," "dimension," "demon")
- Explaining the exact origins of the Apertures
- Dry incident reports or internal memos with no human story. Every broadcast must carry a specific narrative about specific people, told through the advisory

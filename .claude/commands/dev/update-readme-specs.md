---
description: Regenerate the README "Specs" section from openspec/specs/
---

Update the `## Specs` section in `README.md` so it lists every long-lived behavioral
spec under `openspec/specs/`, each with a short one-line description.

**Steps**

1. **Find all specs**

   Glob `openspec/specs/*/spec.md`. Each subdirectory under `openspec/specs/` is one
   capability/spec. Sort the results alphabetically by directory name.

2. **Extract a title and description for each spec**

   For each `openspec/specs/<name>/spec.md`:
   - **Title**: the text after `# Spec: ` on the first heading line. If the file has
     no `# Spec: ...` heading, derive a title from `<name>` (title-cased, hyphens to
     spaces).
   - **Description**: a single concise sentence (~100-120 chars, no line breaks)
     summarizing the spec, derived from its `## Purpose` paragraph. If there is no
     `## Purpose` section, summarize the spec's main requirement(s) in one sentence
     instead.

3. **Build the Specs section**

   Generate a section formatted like this:

   ```markdown
   ## Specs

   Long-lived behavioral specs live in [`openspec/specs/`](openspec/specs/) and are
   the source of truth for how each feature works.

   | Spec | Description |
   |---|---|
   | [Title](openspec/specs/<name>/spec.md) | One-line description |
   ```

   Include one table row per spec directory found in step 1, in the same sorted order.

4. **Insert or update the section in `README.md`**

   - If a `## Specs` section already exists, replace its full contents (from the
     `## Specs` heading up to, but not including, the next `## ` heading or end of
     file) with the newly generated section.
   - Otherwise, insert the new `## Specs` section immediately before the
     `## Development` section. If there is no `## Development` section, insert it
     before `## Documentation`. If neither exists, append it at the end of the file.
   - Keep exactly one blank line before and after the section.

5. **Report**

   Summarize how many specs were listed and whether the section was created or
   updated.

**Guardrails**
- Only edit `README.md`. Do not modify any files under `openspec/`.
- Descriptions are one plain sentence each; inline code spans (`` `like this` ``)
  are fine, but no other markdown formatting inside table cells.
- This command is idempotent — re-running it replaces the existing `## Specs`
  section instead of duplicating it.

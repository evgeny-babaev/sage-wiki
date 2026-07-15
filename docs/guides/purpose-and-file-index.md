# Purpose-Aware Compilation and File Index

sage-wiki can compile the same sources from a chosen operational angle and expose the result as a plain Markdown graph for file-based agents.

## `purpose.md`

`sage-wiki init` creates a comment-only `purpose.md` in the project root. Replace the comments with a concise description of the users, decisions, or outcomes the wiki should support:

```markdown
# Purpose

Help the product team decide what to build, in what order, and why.
Preserve decisions, constraints, ownership, dependencies, and unresolved questions.
```

The effective content is appended to every LLM prompt in summarize, extract, and write, including image summaries, chunk summaries, and hierarchical synthesis. Custom prompt templates receive the same appended instruction.

The compiler adds this exact contract:

```text
WIKI PURPOSE
--- purpose ---
<contents of purpose.md>
--- end purpose ---

Use the wiki purpose as the selection and emphasis criterion for this task. Prioritize information that serves it while preserving source fidelity and explicit uncertainty. The wiki purpose guides selection; it is not source evidence. Do not invent facts, and follow the required output format exactly.
```

Missing, empty, and comment-only `purpose.md` files disable purpose-aware behavior. Existing projects without the file keep their previous prompt behavior.

Remote agents can manage the file through MCP: `wiki_get_purpose` returns its content and effective hash, while `wiki_set_purpose` atomically replaces the complete Markdown file. Updating the file does not compile automatically; run a full compile when the new purpose should take effect.

## Purpose Changes

The manifest stores a hash of the effective purpose. When it changes or the file is removed, the next compile:

1. Marks unchanged sources as modified for the purpose-dependent passes.
2. Bypasses existing summary reuse.
3. Rebuilds summaries, concepts, articles, ontology entries, and their search records.
4. Preserves raw sources, raw indexes, and user-created outputs.
5. Uses standard mode for that full rebuild even when batch mode is configured.

The old manifest, SQLite database, summaries, concepts, and index are backed up before the rebuild. A partial or interrupted rebuild is rolled back and retried on the next compile.

`re-extract` and compile-on-demand reject a stale purpose because they cannot safely rebuild the whole knowledge base. Run `sage-wiki compile` first. Watch mode, CLI/MCP diff, TUI watch, and cost estimation also track `purpose.md`.

## `wiki/index.md`

Every compile, including a no-op compile, generates `<output>/index.md`. The file is deterministic and contains:

- a link to `purpose.md` when purpose-aware compilation is enabled;
- sorted links to existing concept articles;
- sorted links from source paths to existing summaries;
- explicit empty states when nothing has been compiled.

Links are relative to the configured output directory, so nested outputs remain portable. The generator omits missing artifacts instead of producing broken links.

`wiki/index.md` is generated output. Do not edit it manually.

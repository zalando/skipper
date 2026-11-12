# Issue tracker: Local Markdown

Tickets live in this repository, not in GitHub Issues.

## Conventions

- One feature per directory: `.scratch/<feature-slug>/`.
- One ticket per file:
  `.scratch/<feature-slug>/issues/<NN>-<slug>.md`.
- Number tickets from `01` in dependency order, with blockers first.
- Each ticket records its behavior, acceptance criteria, blocking tickets,
  and `Status`.
- Use the status vocabulary in `triage-labels.md`.
- Record dependencies under `Blocked by`, using ticket numbers and titles.
- A ticket can start when all its blockers are complete.
- Append discussion under a `## Comments` heading.
- Reference an existing design document rather than duplicate it.
- A source GitHub issue is context; local ticket publication does not modify it.

## Skill operations

When a skill says “publish to the issue tracker,” create local ticket files.
When it says “fetch the relevant ticket,” read the referenced local file.
Confirm the proposed ticket breakdown before publishing it.

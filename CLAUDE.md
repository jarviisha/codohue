# Claude Code Instructions

@AGENTS.md

## Claude Code

- Treat `AGENTS.md` as the canonical source for shared repository instructions.
- Consult `ARCHITECTURE.md` when a task changes system design, storage, data flow,
  migrations, authentication, operational endpoints, or REST API contracts.
- Consult `README.md`, `.env.example`, and the `Makefile` for setup, configuration,
  and executable commands instead of duplicating them here.
- Use the matching skill under `.claude/skills/` for Spec Kit workflows.
- Keep Claude-specific guidance in this file; put guidance shared with Codex in
  `AGENTS.md`.

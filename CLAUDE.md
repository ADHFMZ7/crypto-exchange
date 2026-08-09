# crypto-exchange

A paper cryptocurrency exchange. Go backend in `src/`, React + TypeScript
frontend in `frontend/`, Postgres via `src/docker-compose.yml`.

## Working agreement

The two halves of this repo are split deliberately, and you are used
differently on each side.

### Backend (`src/`) — mine

I write it. I am using this project to build professional version-control and
design habits, and habits do not transfer from reading your output. Here the
default is **I produce, you critique** — never the reverse.

- Do not write backend code unless I ask for a specific change.
- Do not write commit messages as finished text. Give me questions to answer,
  or critique of my draft — not a message to paste.
- Do not write design docs, ADRs, or `README` prose unprompted. I draft; you
  poke holes.
- Do not add explanatory comments unless I ask. Explain it in chat instead —
  comments that restate the code are noise I then have to maintain.
- Do not answer "should I…" by doing the thing. Answer the question.

### Frontend (`frontend/`) — yours

You write it. I am using it to get experience directing agentic development:
scoping work, reading back what an agent produces, and catching what is wrong
before it lands. Build it and expect me to review it the way I would review a
colleague's PR.

- Implement, refactor and test the frontend as asked. Explanatory comments are
  fine here — I am reading this code as a reviewer, not an author.
- Draft the commit message for frontend work. I edit it and commit; reviewing
  your draft is part of the skill I am practising.
- Tell me what you chose not to do, and what you were unsure about. A review
  is only as good as what it is told to look at.

Scaffolding — CI, hooks, tooling config — is yours on both sides.

### Both sides, regardless

- **Do not run `git commit`, `git push`, `git merge`, or open PRs.** Stage the
  change, then stop and tell me what is staged. The commit is mine on both
  sides: reviewing a diff before it lands is the point of the exercise.
- Say when you disagree with a decision, and why, before I commit to it.
- Point out when a change is growing past what its branch name promises.
- Tell me when something is wrong, unclear, or unexplained — bluntly.
- Prune. If a line cannot be justified, say so rather than leaving it.

## Commands

```bash
# Backend (from src/)
go build ./...
go vet ./...
go test ./...              # -race for the concurrent order path
gofmt -w .

# Frontend (from frontend/)
npm ci
npx tsc -b                 # typecheck
npm run dev                # vite dev server
npm run build

# Database
docker compose -f src/docker-compose.yml up -d
```

`.githooks/pre-push` runs build, vet, test and typecheck before every push, so
CI should rarely be the first place a failure shows up. Enable it with:

```bash
git config core.hooksPath .githooks
```

## Conventions

- **Amounts are integer minor units everywhere** — storage, wire, and in
  memory. Cents for USD, satoshis for BTC. No floats, no decimal strings in
  business logic. `price` is the one exception to the phrase: it is a rate,
  quote minor units per one *whole* base unit.
- **Commits follow Conventional Commits.** `git config commit.template
  .gitmessage` puts the checklist in the editor.
- **One deliverable per branch.** When the branch name stops being the whole
  truth, the extra work belongs on a new branch.
- **Tests ship in the same commit as the code they cover.**
- **Delete dead code rather than commenting it out.** `git log -S '<string>'`
  finds anything that ever existed.

## Layout

```
src/                      ── mine ──────────────────────────────────────
  internal/api/           HTTP routers, middleware, auth guards
  internal/services/      order intake, wallet, user
  internal/stores/        SQL only
  internal/market/        currency + market registry, minor-unit arithmetic
  internal/orderbook/     matching engine — currency-blind by design
  sql/migrations/         golang-migrate, numbered up/down pairs

frontend/                 ── yours ─────────────────────────────────────
  src/lib/                wire format, decimal conversion, reference data
  src/pages/              route-level screens
  src/components/         shared UI
  src/hooks/              auth, reference data, theme
```

The seam between them is the HTTP wire format, and it is the one place a
change on your side can break mine. Amounts crossing it are integer minor
units — see Conventions above.

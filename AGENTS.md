AGENTS.md — Magnus Hire Pilot

Project Context

Before making changes, read docs/PROJECT_CONTEXT.md. Treat it as the product and technical reference, while verifying the current repository because some items in it are planned or historically reported rather than confirmed.

Working Rules

Inspect the existing repository, relevant code, migrations, configuration, and tests before editing.

Preserve existing architecture and working functionality. Do not rewrite components or introduce unnecessary complexity.

Keep the backend as one cohesive Go application. Do not introduce microservices.

Use the existing frontend, Go backend, PostgreSQL, and Docker Compose conventions. Do not redesign or change the frontend unless the task explicitly requests it.

Follow established naming, formatting, package, API, error-handling, and dependency patterns in the repository.

Do not hardcode secrets, credentials, tokens, or environment-specific configuration. Use the project's existing configuration approach.

Enforce authentication, authorization, and user-ownership checks wherever user-owned data is accessed. Never expose passwords, tokens, or sensitive data in logs or API responses.

Preserve data and backward compatibility unless a change is explicitly requested and its impact is understood.

Database and Migration Safety

Inspect the current schema and migration state before proposing database changes.

Never edit a migration that may already have been applied. Create a new migration for every subsequent schema change.

Never drop tables, truncate data, reset database volumes, or run destructive migration commands unless the user explicitly approves the exact operation.

In particular, do not run docker compose down -v unless the user explicitly confirms that deleting the Compose-managed volumes and their data is intended.

Use appropriate constraints, indexes, foreign keys, and transactions.

Implementation Expectations

Implement the requested feature rather than stopping at a plan when implementation is requested.

Keep changes focused; avoid unrelated refactoring and unnecessary dependencies.

Add or update unit and integration tests for relevant behavior and edge cases.

Run applicable formatting, tests, and builds. Report commands and actual outcomes accurately; do not claim checks passed if they were not run.

Update documentation, including the README, when setup, commands, configuration, or workflows change.

For AI-generated career content, use verified profile and job information. Never fabricate skills, employers, titles, dates, education, achievements, or experience.

Keep AI providers and models configurable by workflow stage. Do not hardcode a single provider or model throughout the application.

Do not submit job applications without the user's configured permission and required review or approval.

Working Process

For each task:

Summarize the relevant existing implementation and constraints.

Identify the smallest safe change that satisfies the request.

Implement the change using existing project conventions.

Run relevant tests and validation.

Report:

files changed;

behavior implemented;

migrations or configuration changes;

tests/builds run and their results;

anything not verified or requiring the user's decision.

If the repository conflicts with this file or docs/PROJECT_CONTEXT.md, do not guess. Explain the conflict and ask for clarification when it affects implementation.

Communication

Explain technical decisions clearly and concisely.

Prefer small, verifiable development steps.

Do not assume a feature exists merely because it appears in the product context or roadmap.
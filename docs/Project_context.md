MAGNUS HIRE PILOT

PRODUCT CONTEXT AND DEVELOPMENT SUMMARY

Last updated: 31 October 2026



1. PURPOSE OF THIS DOCUMENT



This document captures the product vision, previously discussed requirements, technical direction, current implementation status, and development decisions for Magnus Hire Pilot. It is intended to be reusable context for future ChatGPT conversations or development sessions.



Important: This document distinguishes between:

- Confirmed implementation: work explicitly reported as completed and/or verified in the conversation.

- Planned direction: product ideas and requirements discussed but not yet confirmed as implemented.

- Unknown status: items that require inspection of the current repository.



Do not assume that every planned feature already exists. When continuing development, inspect the actual repository and verify its current state before changing code.



2. PRODUCT VISION



Magnus Hire Pilot is intended to be an AI-powered job discovery, job matching, and job application automation platform.



The goal is to help a user move through the job-search lifecycle from a professional profile to relevant job opportunities and, where permitted, completed applications.



The intended high-level workflow is:



1. A user creates an account and signs in.

2. The user creates and manages one or more professional profiles.

3. Each profile contains career information such as experience, skills, projects, education, achievements, preferred roles, locations, salary expectations, and other relevant details.

4. The user configures job-search preferences for a profile.

5. The platform discovers jobs from supported job sources.

6. Jobs are normalized into a consistent internal format.

7. The platform evaluates how well each job matches the selected profile.

8. The platform explains or scores the match using relevant criteria.

9. The platform generates a job-specific tailored resume.

10. The platform generates a tailored cover letter when appropriate.

11. The user reviews the job, match, resume, and cover letter.

12. The platform can assist with or automate the application process, subject to user-configured permissions and the capabilities and terms of each job source.

13. The platform tracks application status and relevant history.



The product should reduce repetitive job-search work while keeping the user in control of important decisions, especially before submitting applications.



3. CORE PRODUCT CONCEPTS



3.1 User accounts



The platform should support secure user registration, login, logout, and authenticated access to user-owned data.



Authentication should be implemented in the existing backend rather than by introducing separate microservices.



Security expectations include:

- Secure password hashing. Never store plaintext passwords.

- Input validation and appropriate error responses.

- Authentication middleware for protected endpoints.

- Correct ownership checks so users can only access their own profiles, preferences, resumes, and application data.

- Secrets and credentials must be supplied through environment configuration, not hardcoded.

- Avoid exposing sensitive information in logs or API responses.



The exact authentication method, token strategy, password-reset flow, email verification, and session-management design still need to be confirmed against the repository and product requirements.



3.2 Multiple professional profiles



A single user should be able to create and manage multiple professional profiles.



Examples could include:

- Go Backend Engineer

- Unity Game Developer

- Engineering Lead

- CTO / Technical Leadership



Each profile should be independently usable for job discovery and matching. A user should be able to select which profile is used for a particular search or application.



A profile may contain structured information such as:

- Profile name and target role.

- Professional summary.

- Work experience and responsibilities.

- Technical and domain skills.

- Projects and achievements.

- Education and certifications.

- Years of experience.

- Preferred job titles and role categories.

- Preferred countries, cities, and remote-work preferences.

- Salary expectations and currency.

- Work authorization or visa-related preferences, where relevant.

- Resume source information and generated resume versions.



The final data model should be based on the current schema and product decisions. Do not invent or duplicate tables before inspecting existing migrations.



3.3 Job-search configurations



Users should be able to configure job-search criteria, potentially separately for each profile.



Previously discussed configuration examples include:

- Search countries and cities.

- Remote-only preference.

- Minimum salary.

- Target job titles and role categories.

- Relevant skills or keywords.

- Employment type and other filters, if supported by the chosen job sources.



The system should make it clear which settings belong to the user globally and which belong to an individual profile or search configuration.



4. JOB DISCOVERY, MATCHING, AND APPLICATION WORKFLOW



4.1 Job discovery



The platform is intended to discover relevant jobs from supported job boards and other permitted sources.



The discovery layer should be extensible so that different job sources can be added without rewriting the entire application.



A job ingestion pipeline should:

- Retrieve job information from supported sources.

- Normalize source-specific data into a common internal job representation.

- Preserve source identifiers and original job URLs.

- Handle missing or inconsistent fields.

- Detect duplicates where practical.

- Record when a job was discovered and when it was last updated.

- Handle source errors, rate limits, and retries responsibly.



Do not assume that every job board permits automated scraping or automated applications. Use supported APIs or compliant integrations where available, and respect each source's terms and technical restrictions.



4.2 Job matching and scoring



The project has previously discussed a weighted job-matching approach with these initial weights:



- Technical Skills: 35%

- Experience: 25%

- Role Alignment: 15%

- Location: 10%

- Visa Eligibility: 10%

- Salary Fit: 5%



These weights total 100%.



Treat these as the previously proposed baseline, not as immutable rules. They should be configurable or revisited as the product evolves.



The matching system should:

- Compare a job description against the selected professional profile.

- Produce a transparent match score.

- Explain important matching strengths and gaps.

- Avoid claiming that a profile contains skills or experience that it does not contain.

- Distinguish confirmed profile facts from inferred or uncertain matches.

- Make location, salary, and work-authorization constraints visible.



4.3 Tailored resume generation



For a selected job and profile, the system should generate a job-specific resume using the user's real career information.



The resume-generation process should:

- Reorder or emphasize relevant existing experience and skills.

- Tailor summaries and descriptions to the job requirements.

- Preserve factual accuracy.

- Never invent employers, job titles, dates, education, certifications, achievements, or skills.

- Keep the original profile data separate from generated resume versions.

- Store or otherwise make generated versions available for review and reuse.



A generated resume should be associated with the relevant profile and job/application context.



4.4 Tailored cover letters



The platform should generate a cover letter based on:

- The selected professional profile.

- The job description.

- The company and role information available.

- The user's genuine experience and achievements.



Cover letters must not fabricate personal experience or company knowledge.



4.5 Application assistance and automation



The long-term vision includes applying to suitable jobs automatically or semi-automatically.



The intended workflow should support:

- Preparing application materials.

- Mapping profile information to application form fields.

- Filling supported forms where technically and legally permitted.

- Showing the user the final application details.

- Requiring user review or approval when configured, especially before final submission.

- Recording the application outcome and status.

- Avoiding duplicate applications.



The precise boundary between fully automatic submission and user-approved submission remains a product decision. Build this in a controlled, auditable way rather than silently submitting applications.



5. AI AND LOCAL LLM STRATEGY



A major product requirement is the ability to use local AI models through Ollama, while retaining the flexibility to use external AI providers when desired.



The system should not be tightly coupled to one model or provider.



5.1 Provider flexibility



Previously discussed providers and options include:

- Ollama for locally hosted models.

- OpenAI models.

- Anthropic Claude.

- Google Gemini.



The existing ApplyPilot-related work has included provider support for Claude, Gemini, OpenAI, and Ollama. However, the exact code and provider support in the Magnus Hire Pilot repository must be verified independently. Do not assume that code from another project or an earlier prototype has already been integrated.



5.2 Per-stage model configuration



A key requirement is to configure which AI provider and model is used at each stage of the workflow.



Potential AI stages include:

- Job description parsing and normalization.

- Job/profile matching and explanation.

- Resume tailoring.

- Cover-letter generation.

- Application-question drafting.

- Other AI-assisted workflow steps introduced later.



For example, a user might choose a local Ollama model for job-description analysis and a hosted model for resume generation. This is an example of the desired flexibility, not a fixed default.



The architecture should therefore provide:

- A provider abstraction.

- A model configuration for each AI task or workflow stage.

- Provider-specific configuration stored securely.

- A common interface for invoking models.

- Consistent error handling, timeouts, and retry policies.

- Visibility into which provider/model was used for a task.

- Sensible handling when a local model is unavailable.

- User control over local versus hosted inference.



Do not hardcode one model name throughout the application.



5.3 Local-first considerations



Ollama is important because the user wants the option to run AI locally.



The design should account for:

- Ollama base URL and model configuration.

- Model availability checks.

- Connection failures and timeouts.

- The possibility that a local model has limited memory or performance.

- Clear distinction between local and external provider calls.

- Avoiding accidental transmission of private profile data to an external provider unless the user has configured and permitted it.



The exact default model should be configurable and should not be assumed without checking the user's current Ollama setup.



6. TECHNICAL ARCHITECTURE



6.1 Architectural direction



The user prefers a straightforward service-based application deployed with Docker Compose, rather than splitting the product into microservices.



The intended deployment is a single application composed of a small number of containers, for example:

- Frontend container.

- Go backend container.

- PostgreSQL database container.



These are deployment services in Docker Compose; this does not mean the backend should be decomposed into independently deployed microservices.



Keep the backend as one cohesive Go application. Internally, organize code into clear packages or modules, but avoid unnecessary distributed-system complexity.



6.2 Backend



The backend is based on Go.



Previously discussed backend direction:

- One Go API service.

- PostgreSQL as the primary relational database.

- REST APIs for frontend communication, unless the existing repository establishes another convention.

- Authentication and authorization in the same backend.

- Business logic organized into maintainable packages.

- Database access and migrations managed within the Go project.

- Environment-based configuration.

- Health and readiness endpoints.



The exact Go web framework, routing conventions, dependency-injection approach, and package structure should be determined by inspecting the existing repository. Preserve established patterns where reasonable.



6.3 Frontend



The frontend is intended to be a separate application/container that communicates with the Go backend.



The user has emphasized that the frontend should provide a usable product interface, not merely an API demonstration.



The full visual design and detailed screen specifications have not been conclusively captured in this conversation. They should be discussed and documented before substantial UI redesign.



At a minimum, the product experience will need to support the main workflows:

- Registration and login.

- Dashboard and job-search overview.

- Professional profile creation and editing.

- Multiple profile management and profile selection.

- Job-search configuration.

- Job discovery and job listing.

- Job details and match explanations.

- Tailored resume review.

- Cover-letter review.

- Application review and submission/approval.

- Application tracking and history.

- AI provider and per-stage model configuration.



Do not assume this is the final navigation or visual design. Treat it as a functional screen inventory to refine.



The existing frontend UI was explicitly to remain unchanged during the initial Docker and database foundation work. Later UI changes should be deliberate and coordinated with the user.



6.4 PostgreSQL



PostgreSQL is the selected database.



Database configuration should use environment variables. Do not hardcode credentials or commit real secrets.



The application should use explicit migrations for schema changes. Existing applied migrations must not be edited to introduce new schema changes; create new migrations instead.



Use appropriate:

- Primary keys.

- Foreign keys.

- Unique constraints.

- Check constraints where useful.

- Indexes for common queries.

- Transactions where safe.

- Referential integrity and ownership constraints.



6.5 Docker Compose



The project has been moved toward a Docker Compose development setup.



The intended development setup includes:

- PostgreSQL service.

- Go backend service.

- Frontend service.



The backend should wait for PostgreSQL to become healthy before starting when using Compose health checks and dependency conditions.



The frontend should communicate with the backend using the appropriate Compose service hostname or configured public API URL. The backend should connect to PostgreSQL using the database service name and the container's internal PostgreSQL port, not localhost or the host-published port.



A host-port conflict occurred because port 5432 was already in use. The discussed solution was to publish PostgreSQL on host port 5433 while retaining port 5432 inside the database container. The backend-to-database connection should still use the Compose service hostname and port 5432.



Do not run destructive volume-reset commands casually. In particular, `docker compose down -v` removes the Compose-managed database volume and can delete development database data.



7. DATABASE MIGRATION SYSTEM



The user rejected the earlier migration approach and requested a custom Go migration runner exposed through Cobra commands.



The repository inspection report described the following implementation:



- A custom Go migration runner in `backend/internal/migrate/migrate.go`.

- Paired, versioned SQL migration files in `backend/migrations`.

- Up migrations run in ascending order.

- Down migrations run in descending order.

- Applied migration versions and names are recorded in `hirepilot_schema_migrations`.

- The server does not automatically execute migrations at startup.

- The Compose setup runs migrations as a separate command.

- The available commands were reported as `migrate up`, `migrate down`, `migrate status`, and `migrate create`.



The report identified these migration files:

- `000001_initial`

- `000002_create_teams_table`



The report also described:

- Normal migration SQL and its tracking update run in a transaction.

- A `-- migrate:no-transaction` directive opts out of the transaction.

- The migration runner uses a PostgreSQL advisory lock.

- The status command may create the tracking table with `CREATE TABLE IF NOT EXISTS`, so it is not strictly read-only.

- Migration checksums were not implemented at the time of that report. Therefore, edits to an already-applied migration with the same name might not be detected.



The user ran:

`docker compose run --rm backend migrate status`



The output was:

`000001_initial pending`

`000002_create_teams_table pending`



The user then ran:

`docker compose run --rm backend migrate up`



The command returned:

`Migrations applied.`



The user subsequently reported that the database tables were visible in the database client.



This confirms that the migration command reported success and the user observed the tables. A fresh `migrate status` output was requested but is not present in the captured conversation. Verify the current migration state before relying on it.



Important safety rules:

- Never edit migrations that have already been applied.

- Add a new migration for subsequent schema changes.

- Do not run `migrate down` unless the user understands which schema/data will be removed.

- Do not run `docker compose down -v` unless the user explicitly intends to delete the development database volume.

- Consider adding migration checksums and tests if they are still absent.

- Make success and failure output clear and actionable.



8. IMPLEMENTATION HISTORY AND CONFIRMED PROGRESS



The following progress was reported during the conversation.



8.1 Initial repository and Docker setup



Codex reported creating a two-service development setup:

- PostgreSQL.

- Go backend.



The backend waits for PostgreSQL to become healthy. Migrations were configured to run only when explicitly requested.



The README was updated with instructions for:

- Starting the services.

- Running migrations.

- Checking health and readiness.

- Direct host development.

- Stopping or resetting the development database.



The initial migration was reported to create tables in dependency order, with a down migration that drops them in reverse order.



Codex validated the Compose configuration using:

`docker compose config`



It also ran:

`git diff --check`



At that stage, Codex had not started the containers or applied migrations.



8.2 Frontend container



The user later requested that the frontend be added to the existing Docker Compose setup without changing the backend or database.



Codex reported building/recreating:

- `magnas_hirepilot-backend`

- `magnas_hirepilot-frontend`

- `magnas_hirepilot-db`



A PostgreSQL host-port conflict occurred on 5432. The intended fix was to map host port 5433 to container port 5432.



The user subsequently reported that Docker Compose was running and the frontend UI could be opened.



The exact current Compose configuration and port mappings should still be checked in the repository.



8.3 Migration setup and execution



The user chose to use the custom Go/Cobra migration approach and removed/replaced the earlier migration setup. A later inspection report confirmed the custom Go SQL-file runner described in Section 7.



The user ran the migration status command, saw both migrations pending, then ran migration up and received `Migrations applied.` The user reported seeing the created tables.



8.4 Authentication and profiles



The next proposed development phase was secure user registration, login, and multiple professional profiles.



A detailed implementation prompt was started but intentionally paused when the user requested a complete reusable product-context summary instead.



Therefore, do not assume registration, login, or profile APIs have already been implemented.



9. APPLY PILOT AS A REFERENCE



The product discussions refer to ApplyPilot as a reference for job discovery, ingestion, matching, AI-assisted resume tailoring, and provider integration.



Earlier ApplyPilot-related project context included:

- A local-first CLI prototype written in Go.

- A master career profile in JSON/YAML.

- Job-description ingestion and normalization.

- Weighted matching and scoring.

- Resume tailoring work that was previously incomplete or stubbed in that prototype.

- Local Ollama integration.

- Provider options including Claude, Gemini, OpenAI, and Ollama.

- Search configuration such as countries, cities, minimum salary, and remote-only preferences.



The earlier matching weights were:

- Technical Skills: 35%

- Experience: 25%

- Role Alignment: 15%

- Location: 10%

- Visa Eligibility: 10%

- Salary Fit: 5%



There were also historical prototype issues, including an empty default SQLite database while other database files contained job records, zero-byte tailored-resume outputs from failed runs, Gemini quota exhaustion, a model identifier issue, and missing retry/backoff handling for rate limits.



These are historical notes from a separate ApplyPilot-related development effort. They should not be treated as confirmed defects in the current Magnus Hire Pilot repository. Reuse ideas and proven concepts where appropriate, but inspect and selectively port code rather than assuming the projects share the same implementation.



10. DEVELOPMENT PRINCIPLES AND CONSTRAINTS



When making changes to Magnus Hire Pilot:



1. Inspect the existing repository before editing files.

2. Report the current architecture and implementation status.

3. Preserve working functionality.

4. Do not introduce microservices; keep one cohesive Go backend.

5. Keep frontend, backend, and PostgreSQL as Docker Compose services where appropriate.

6. Reuse the existing PostgreSQL connection and configuration.

7. Do not hardcode secrets or credentials.

8. Do not delete database data, migrations, or existing tables.

9. Never modify migrations that have already been applied.

10. Add new migrations for schema changes.

11. Avoid destructive commands unless explicitly approved.

12. Implement working code rather than only producing a plan when implementation is requested.

13. Add tests for important business logic and edge cases.

14. Run formatting, build, and relevant tests.

15. Update the README when commands, setup, or workflows change.

16. Clearly report what was changed, what was tested, and what could not be tested.

17. Distinguish between a successful build, a successful migration command, and verified application behavior.

18. Keep AI-generated career content grounded in the user's actual profile.

19. Make AI provider and model selection configurable by workflow stage.

20. Keep the user in control of application submission unless a specific automation policy is explicitly configured.



11. SUGGESTED HIGH-LEVEL DEVELOPMENT ROADMAP



The following is a proposed sequence. It is not a claim that these phases are already complete.



Phase 0 - Repository and architecture baseline

- Inspect the repository.

- Confirm frontend framework and current screens.

- Confirm Go backend structure and framework.

- Confirm PostgreSQL schema and migration state.

- Confirm Docker Compose services, ports, volumes, and environment variables.

- Document how to run the complete application locally.



Phase 1 - Authentication and user foundation

- Implement user registration.

- Implement login and logout.

- Add secure password hashing and authentication middleware.

- Add authorization and user ownership checks.

- Add tests and API documentation.



Phase 2 - Multiple professional profiles

- Implement profile creation, retrieval, update, and deletion.

- Support multiple profiles per user.

- Add structured profile sections for experience, skills, projects, education, and preferences.

- Ensure all profile operations enforce user ownership.



Phase 3 - Job-search configuration

- Add configurable search criteria.

- Support country, city, remote preference, salary, role, and keyword filters.

- Associate configurations with the correct user/profile.



Phase 4 - Job source integrations and discovery

- Define a common job-source interface.

- Integrate permitted and supported sources.

- Ingest and normalize job data.

- Handle duplicate detection, retries, rate limits, and source failures.

- Track source URLs and identifiers.



Phase 5 - Job matching and scoring

- Match jobs against a selected professional profile.

- Implement the weighted scoring model.

- Explain strengths, gaps, and constraints.

- Add tests and make scoring rules configurable where appropriate.



Phase 6 - AI provider and model configuration

- Implement provider abstraction for Ollama and selected hosted providers.

- Add per-stage model configuration.

- Store provider settings securely.

- Add connection/model availability checks and robust error handling.

- Make local versus hosted inference explicit.



Phase 7 - Tailored resumes

- Generate job-specific resumes from verified profile data.

- Preserve factual accuracy.

- Store generated versions and associate them with jobs/applications.

- Provide review and editing workflows.



Phase 8 - Tailored cover letters

- Generate job-specific cover letters from profile and job data.

- Prevent fabricated claims.

- Store and review generated versions.



Phase 9 - Application workflow

- Prepare application data and materials.

- Support form mapping and assisted form completion where permitted.

- Add user review/approval controls.

- Submit only according to the configured permission model.

- Track application status and prevent duplicate submissions.



Phase 10 - Dashboard, tracking, and product refinement

- Add a unified view of discovered jobs, matches, generated materials, and applications.

- Add filters, history, status updates, and useful metrics.

- Improve usability based on real workflow testing.



12. OPEN PRODUCT AND DESIGN QUESTIONS



These topics need future discussion and explicit decisions:



- What is the preferred frontend framework and component system in the current repository?

- What should the final dashboard layout and navigation look like?

- Should profiles be created manually, imported from an existing resume, or both?

- Which profile fields are mandatory versus optional?

- Which job sources should be supported first?

- Which sources permit automated ingestion and application workflows?

- Should job searches run on demand, on a schedule, or both?

- Should AI configuration be global, per user, per profile, and/or per workflow stage?

- Which AI provider/model should be the default for each stage?

- What should happen when the local Ollama model is unavailable?

- Should generated resumes and cover letters require approval before use?

- Should final job applications always require user approval, or can users explicitly enable automatic submission for selected sources?

- What application statuses and tracking events are required?

- What retention and privacy rules should apply to resumes, profile data, credentials, and application history?



13. CURRENT NEXT STEP



The last confirmed development milestone in the captured discussion is:

- Docker Compose was running with backend, database, and frontend services, and the user could open the frontend UI.

- The custom Go migration command reported successful application of the initial migrations.

- The user reported that the database tables were visible.

- Authentication, registration, and multiple-profile APIs were the proposed next development phase, but implementation was not confirmed.



Recommended next action:

Ask Codex to inspect the current Magnus Hire Pilot repository and report the actual state of the frontend, backend, database schema, migrations, and Docker Compose setup. Then plan the authentication and multi-profile foundation based on the real code.



Do not immediately rewrite the project or assume the earlier reports are still accurate. Confirm the current working tree and database state first.



14. REUSABLE INSTRUCTION FOR A FUTURE CHATGPT CONVERSATION



You are helping me continue the development of Magnus Hire Pilot, an AI-powered job discovery and application automation platform.



Use this document as the product and technical context. Preserve the distinction between completed work, planned features, and unknown implementation status. Do not assume that a feature is implemented just because it appears in this document.



My preferred architecture is a simple Docker Compose-based application with a frontend container, one cohesive Go backend service, and a PostgreSQL database. I do not want unnecessary microservices.



The product should support user accounts, multiple professional profiles, configurable job searches, job discovery and normalization, transparent job/profile matching, AI-tailored resumes, AI-tailored cover letters, and an assisted or configurable automated application workflow.



A major requirement is flexible AI integration. I want to use local models through Ollama and optionally use hosted providers such as OpenAI, Claude, or Gemini. I want to configure the provider and model separately for different workflow stages rather than hardcoding one model for the whole product.



ApplyPilot is a reference for useful job-search, ingestion, matching, and AI workflow concepts. Treat it as a reference project, not proof that its code is already part of Magnus Hire Pilot.



Before proposing or making code changes:

1. Inspect the current repository.

2. Summarize what is already implemented.

3. Identify gaps relative to the product requirements.

4. Propose a small, safe implementation step.

5. Preserve existing functionality and data.

6. Never modify applied migrations; create new migrations.

7. Avoid destructive database or Docker volume commands without explicit approval.

8. Implement and test the agreed work, then report exact results.



Continue in small, verifiable phases. Keep explanations and implementation prompts in English. When I ask for a Codex prompt, make it fully English and directly copyable.
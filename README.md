# docviewer-go

Backend for **Scenario D: Unified Document Viewer**. Given a VIN, the API calls the
Sales and Service systems in parallel and returns one list of documents, newest first,
each marked with the system it came from.

- If one system is slow or down, you still get the other system's documents, with `partial: true`.
- Results are cached per system in Postgres for 15 minutes. If a system fails and an older
  snapshot exists, that snapshot is served with `stale: true`.
- Every call to a source system is written to an audit table and to the logs.

Design: [docs/design.md](docs/design.md) · API contract: [docs/openapi.yaml](docs/openapi.yaml) · [AI Collaboration Narrative](#ai-collaboration-narrative) · Full AI log: [docs/genai-log.md](docs/genai-log.md)

## Run it (Docker, recommended)

Needs Docker with Compose (Docker Desktop, Colima or plain Docker Engine).

```bash
docker compose up --build
```

This starts Postgres, applies the migrations, starts both mock systems and the API.

| Service | URL |
|---|---|
| API | http://localhost:8080 |
| Mock Sales system | http://localhost:8081 |
| Mock Service system | http://localhost:8082 |
| Postgres | `localhost:5433`, user/password `postgres`, database `docviewer` |

Stop with `docker compose down`. Add `-v` to also delete the database volume.

## Try it

```bash
# Both systems have documents for this VIN: 4 documents from 2 sources
curl -s localhost:8080/api/v1/vehicles/1HGCM82633A004352/documents

# Only the Service system knows this VIN
curl -s localhost:8080/api/v1/vehicles/JH4KA7532NC036794/documents

# Both mock systems return 500 for VINs starting with FAIL: 200 with partial=true and two errors
curl -s localhost:8080/api/v1/vehicles/FAIL0000000000001/documents

# Both mock systems wait 5s for VINs starting with SLOW: the 3s timeout fires, partial=true
curl -s localhost:8080/api/v1/vehicles/SLOW0000000000001/documents

# Invalid VIN: 400
curl -s localhost:8080/api/v1/vehicles/abc/documents

# Health
curl -s localhost:8080/healthz   # liveness
curl -s localhost:8080/readyz    # readiness (Postgres reachable)
```

Run the first request twice. The second answer comes from the snapshot cache, and the
mock systems log no new request.

### Follow one request through all services

Send your own request id, then grep for it. The API forwards it to both systems.

```bash
curl -s -H 'X-Request-ID: demo-42' localhost:8080/api/v1/vehicles/1HGCM82633A004352/documents
docker compose logs --no-log-prefix server mock-sales mock-service | grep demo-42
```

You get, in JSON: the request seen by each mock system, one `source call` line per system
(outcome and duration), a `documents served` summary, and the HTTP access line.

### Audit log

```bash
docker compose exec postgres psql -U postgres -d docviewer \
  -c "SELECT vin, source, outcome, duration_ms, called_at FROM source_call_log ORDER BY id DESC LIMIT 10"
```

## Run it without Docker

Needs Go 1.26+ and a local Postgres.

```bash
createdb docviewer
go install github.com/pressly/goose/v3/cmd/goose@v3.28.0
goose -dir migration postgres "postgres://localhost:5432/docviewer?sslmode=disable" up

# three terminals
PORT=8081 go run ./cmd/mock-sales
PORT=8082 go run ./cmd/mock-service
go run ./cmd/server
```

The server reads these environment variables:

| Variable | Default |
|---|---|
| `PORT` | `8080` |
| `DATABASE_URL` | `postgres://localhost:5432/docviewer?sslmode=disable` |
| `SALES_URL` | `http://localhost:8081` |
| `SERVICE_URL` | `http://localhost:8082` |

## Tests

```bash
go test -race ./...
```

The Postgres integration tests in `internal/store` use `DATABASE_URL` (or the default above)
and skip when no database is reachable. To run them against the Compose database:

```bash
DATABASE_URL="postgres://postgres:postgres@localhost:5433/docviewer?sslmode=disable" go test ./internal/store/
```

## Layout

```
cmd/server          API: wiring, middleware, graceful shutdown
cmd/mock-sales      mock Sales system (port 8081)
cmd/mock-service    mock Service system (port 8082)
internal/domain     Document and the Source interface every system implements
internal/source     HTTP clients for the two systems
internal/aggregator parallel fan-out, timeout, partial failure, cache, call log
internal/store      Postgres: snapshot cache and audit log
internal/api        HTTP handlers, health checks, access log
internal/reqid      request id: middleware, context, log handler
migration           goose SQL migrations
docs                design doc, OpenAPI spec, GenAI log
```

Adding a third system means one new type that implements `domain.Source` and one line in
`cmd/server/main.go`. The aggregator does not change.

## AI Collaboration Narrative

I used Claude Code (in my IDE, with access to the repository and a terminal) as a mentor
and reviewer, not as the author of the system. Every change it made is listed, with what I
checked, in [docs/genai-log.md](docs/genai-log.md). Section 8 of the design doc covers the
design phase in my own words.

### Strategy for guiding the AI

- **Design first, code second.** I wrote `docs/design.md` before any code: components, data
  flow, the partial-failure table, cache and timeout decisions. The AI reviewed it and I added
  or removed points. Once the design existed, it was the frame the AI had to follow; when it
  suggested starting with the mock servers before the design was written, I stopped it.
- **I write the core logic, the AI writes around it.** I wrote the source clients, the
  aggregator (parallel fan-out, timeouts, partial failure, cache, stale fallback, outcome
  classification) and the Postgres store. The AI wrote the parts that follow a pattern I had
  already set or that are mostly setup: copies of my Sales code for the Service system, most
  of the tests, the HTTP wiring, Docker, the observability scaffolding, the OpenAPI spec and
  this README.
- **Small steps.** One piece at a time, each one compiled, tested and committed before the
  next, so a wrong suggestion could not spread across the codebase.

### Verifying and refining its output

- **Review both ways.** The AI reviewed my code and found real bugs, which I fixed myself:
  `cancel()` called before the request it was meant to limit, a `for range` that silently
  dropped empty snapshots, an error built with `fmt.Errorf` but never returned. I read
  everything the AI wrote before committing and asked it to explain what I did not follow
  (middleware chaining, `errors.As`, how a cache hit skips the source calls).
- **Run it, do not trust it.** Claims were checked by running something:
  - the parallel speed-up was measured with a test before and after the change
    (about 200 ms sequential, 100 ms parallel);
  - SQL was run against the real database before going into the code or the doc;
  - the full stack was started with `docker compose` from an empty volume and exercised
    with curl and Postman, including the failing and slow VINs.
- **The AI was wrong too.** It warned that an `http.Client` timeout would not match
  `errors.Is(err, context.DeadlineExceeded)`. A small experiment on Go 1.26 showed that it does, so the simpler check stayed. It also told me the brief did not ask for a video; it had only seen the part of the brief I pasted, and the full PDF showed otherwise. Since then I check its claims about the requirements against the original document.

### Ensuring the final quality

- **Tests on the core logic**, run with the race detector (`go test -race ./...`): partial
  failure, timeouts, empty vs. error, sort order, cache hit / expiry / stale fallback, the
  four call outcomes, and a timing test that fails if the sources ever run sequentially again.
  Postgres behaviour is covered by integration tests against a real database.
- **The design doc matches the code.** Anything promised but not built was removed or moved
  to Future Work (for example the Prometheus endpoint), instead of being left as a claim.
- **End-to-end checks on the real stack** before each commit that touched behaviour: merged
  results, `partial` and `stale` flags, the 3 s timeout, cache hits that never reach the
  source systems, and one request id followed through all three services.
- **A written trail.** [docs/genai-log.md](docs/genai-log.md) records, per day, what I wrote,
  what the AI wrote, and what I reviewed, so the split can be checked against the git history.

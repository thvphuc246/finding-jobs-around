# Finland's IT Employers Seeking Engine

A map of IT employers in Finland. The sync loads the Teknologiateollisuus Member list into Postgres as Companies and Locations and geocodes each Location with Digitransit. It also reads each Company's Teamtailor feed, so every Location shows its Hiring status and open Jobs.

## Run

Start Postgres:

```sh
docker compose up -d --wait
```

Copy `.env.example` to `.env`, fill in `DIGITRANSIT_API_KEY`, then run the sync once:

```sh
set -a; . ./.env; set +a; go run ./cmd/sync
```

A second run with an unchanged Member list geocodes nothing.

Start the API:

```sh
set -a; . ./.env; set +a; go run ./cmd/api
```

Then start the map:

```sh
cd web
bun install
bun dev
```

Open http://localhost:5173.

## Checks

```sh
golangci-lint run
TEST_DATABASE_URL=postgresql://postgres:postgres@localhost:5432/jobs_test go test -race ./...
```

In `web/`:

```sh
bun run lint
bun run typecheck
bun test
bun run build
```

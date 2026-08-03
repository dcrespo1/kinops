# KinOps

A small, self-hosted household operations hub. KinOps combines recurring
chores, fixed or rotating assignments, family events, daily/weekly/monthly
agenda views, per-person calendar feeds, and a session-protected analytics
dashboard.

## Requirements

- Docker with Docker Compose
- `just` and Go 1.26+ only for local development

## Setup with Docker Compose

1. Create your local configuration:

   ```sh
   cp .env.example .env
   ```

2. Generate the admin password hash:

   ```sh
   docker compose run --rm --build kinops hash-password
   ```

   Enter your desired password and copy the generated hash into `.env`. Keep
   the hash inside single quotes because it contains dollar signs:

   ```dotenv
   KINOPS_ADMIN_USERNAME=admin
   KINOPS_ADMIN_PASSWORD_HASH='generated-hash-here'
   KINOPS_ADMIN_COOKIE_SECURE=false
   ```

   Leave `KINOPS_ADMIN_COOKIE_SECURE=false` for normal HTTP access on your
   local network. Set it to `true` only when the browser reaches the app over
   HTTPS.

   Local developers can run `just admin-password-hash` instead.

3. Start the app:

   ```sh
   docker compose up -d --build
   ```

4. Open [http://localhost:8081](http://localhost:8081).

Mealie is available separately at
[http://localhost:9925](http://localhost:9925). On a fresh installation, sign
in with Mealie's documented bootstrap credentials (`changeme@example.com` /
`MyPassword`), change that password immediately, and create a long-lived API
token under `/user/profile/api-tokens`. Put the token in the ignored local
`.env` file as `MEALIE_API_TOKEN`; never commit it.

The SQLite database is stored in the `kinops-data` Docker volume and survives
container recreation.

To stop the app without deleting its data:

```sh
docker compose down
```

Do not add `--volumes` unless you intentionally want to delete the database.

## Initial configuration

1. Open `/people` and add the household members.
2. Open `/chores` to create chores and schedules.
3. Open `/events` to add birthdays, appointments, vacations, and other family
   events.
4. Open `/kitchen/daily` or `/kitchen/weekly` to view the meal plan stored in
   Mealie.
5. Use `/admin` to review analytics and copy each person's calendar feed URL.

Rotating a calendar URL from the admin page immediately invalidates that
person's previous calendar subscription.

## Main routes

- `/daily` — daily assignments and completion controls
- `/weekly` — Monday-to-Sunday schedule
- `/monthly` — monthly calendar
- `/chores` — chore and schedule management
- `/events` — family event management
- `/kitchen/daily` — meals planned for one day
- `/kitchen/weekly` — Monday-to-Sunday meal plan
- `/kitchen/recipes` — search Mealie recipes, manage favorites, and schedule meals
- `/kitchen/groceries` — view and update Mealie shopping lists
- `/people` — household member setup
- `/admin` — protected analytics and calendar administration

## Local development

Create `.env` as described above, then run:

```sh
just bootstrap
just run
```

Useful commands:

```sh
just generate  # regenerate templ files
just test      # run tests with the race detector
just check     # generate, vet, and run all tests
just build     # build ./bin/kinops
```

By default, local data is stored in `./data/kinops.db`.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `KINOPS_PORT` | `8081` | Host port used by Docker Compose |
| `KINOPS_LISTEN_ADDRESS` | `:8081` | Address used by the Go server |
| `KINOPS_DATABASE_PATH` | `./data/kinops.db` | SQLite path outside Docker |
| `KINOPS_TIMEZONE` | `America/New_York` | Household timezone |
| `KINOPS_ADMIN_USERNAME` | unset | Admin login username |
| `KINOPS_ADMIN_PASSWORD_HASH` | unset | Generated admin password hash |
| `KINOPS_ADMIN_COOKIE_SECURE` | `false` | Restrict the session cookie to HTTPS |
| `MEALIE_PORT` | `9925` | Host port for the Mealie UI and API docs |
| `MEALIE_PUBLIC_URL` | `http://localhost:9925` | Browser-reachable Mealie URL |
| `MEALIE_BASE_URL` | unset | KinOps-to-Mealie URL; use `http://mealie:9000` in Compose |
| `MEALIE_API_TOKEN` | unset | Long-lived server-only Mealie token |
| `MEALIE_DEFAULT_SHOPPING_LIST_ID` | unset | Optional preferred Mealie list |
| `MEALIE_REQUEST_TIMEOUT` | `5s` | Timeout for server-side Mealie API requests |

Admin routes are disabled unless both admin credential variables are set.

Kitchen reads use a small in-memory cache. If Mealie becomes temporarily
unavailable after a successful read, KinOps may show the prior meal, recipe,
or grocery data with a visible stale-data warning. Kitchen writes are never
queued or reported as successful until Mealie confirms them. Mealie
connectivity and version are shown on the protected `/admin` page and do not
affect the core `/healthz` check.

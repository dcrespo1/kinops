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
4. Use `/admin` to review analytics and copy each person's calendar feed URL.

Rotating a calendar URL from the admin page immediately invalidates that
person's previous calendar subscription.

## Main routes

- `/daily` — daily assignments and completion controls
- `/weekly` — Monday-to-Sunday schedule
- `/monthly` — monthly calendar
- `/chores` — chore and schedule management
- `/events` — family event management
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

Admin routes are disabled unless both admin credential variables are set.

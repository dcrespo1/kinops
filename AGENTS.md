# AGENTS.md

## Project

KinOps is a self-hosted household operations web application for a family.
Chores, family events, calendar feeds, and household analytics are its current
core capabilities.

## Architecture

- Go monolith
- chi router
- templ templates
- htmx
- Alpine.js
- SQLite via modernc.org/sqlite
- Goose migrations
- `just` is the task runner
- Containerized with Docker Compose
- No CGO
- No frontend build toolchain

## MVP direction

Build the simplest reliable household chore application that looks good on a
desktop or phone and runs as one self-hosted container on a trusted local
network.

The default and only MVP appearance is dark mode. A theme toggle and separate
light palette are outside the MVP.

The MVP provides:

- one global, editable chore list;
- recurring and one-off schedules;
- fixed assignment or strict per-occurrence rotation across active people;
- daily, weekly, and monthly schedule views;
- per-person subscribable ICS calendar feeds;
- append-only completion history and lightweight analytics;
- persistent SQLite data through Docker Compose.

MVP non-goals include multi-household tenancy, public authentication, OAuth,
native mobile applications, rewards, points, and other gamification.

## Engineering preferences

- Prefer the standard library where practical.
- Keep handlers thin.
- Put persistence logic in `internal/store`.
- Put recurrence logic in `internal/scheduling`.
- Use explicit types instead of passing raw strings broadly.
- Return wrapped errors with useful context.
- Avoid global state.
- Keep dependencies minimal.
- Do not add an ORM.
- Do not add JavaScript frameworks.
- Do not change schema decisions without explaining why.

## Testing

- Use Go's standard `testing` package.
- Prefer table-driven tests for recurrence rules.
- Use `httptest` for handlers and routing.
- Use temporary SQLite databases for integration tests.
- Run `just test` after changes.
- Run `just check` before considering a task complete.
- Do not remove or weaken existing tests.

## Commands

- Generate templ: `just generate`
- Generate admin password hash: `just admin-password-hash`
- Run tests: `just test`
- Run checks: `just check`
- Run locally: `just run`
- Development mode: `just dev`

## Phase 2 scope

Implement chore CRUD and the scheduling engine.

Supported recurrence rules:

- daily
- every N days
- specific weekdays
- monthly day-of-month
- one-off date

Assignment modes:

- fixed
- rotate

Rotation is strict per occurrence.

Each generated occurrence has a persistent sequence number and assigned person.

Editing a schedule must preserve historical instances and regenerate only pending future instances.

Monthly dates that do not exist in a month are skipped, not clamped.

Generate instances through 60 days from the household-local current date.

## Phase 3 scope

Implement the Daily View and the core completion interaction.

- `/` and `/daily` show the selected household-local date.
- The default date is the household-local current date.
- Chores are grouped by assigned person.
- Pending occurrences before the selected date are surfaced as overdue.
- Today's pending, done, and skipped occurrences remain visible.
- Completing a pending occurrence is a one-tap htmx interaction.
- A completed occurrence can be reopened to correct an accidental tap.
- Complete and reopen operations are transactional and idempotent.
- Each real transition appends exactly one completion log.
- Completion-log person attribution uses the occurrence's assigned person.
- Completing or reopening never changes assignment, sequence, or rotation.
- Skip and unskip actions are not part of Phase 3.
- The household may contain more than two active people. Fixed schedules may
  target any active person, and rotation proceeds deterministically through all
  active people in person-ID order from the configured starting person.
- Adding a person regenerates pending occurrences for active rotating
  schedules from today so the new person joins rotation immediately. Completed
  and other historical assignments remain unchanged.
- The Daily View must stack cleanly on small screens and retain accessible
  touch targets.

The container must build and run with `docker compose up`, persist SQLite data
on its named volume, run as a non-root user, and report healthy through the
binary's `healthcheck` command.

## Phase 4 scope

Implement Weekly and Monthly schedule views.

- Calendar weeks run Monday through Sunday in the household timezone.
- The desktop Weekly View uses people as rows and days as columns.
- The mobile Weekly View uses day-by-day cards instead of horizontal scrolling.
- Weekly task chips show persisted assignment and completion status.
- The Monthly View uses a fixed Monday-first, six-week calendar grid.
- Monthly cells show scheduled count, completion ratio, and person-colored
  status indicators.
- Pending occurrences before the household-local current date are visually
  overdue.
- Calendar navigation uses strict date or month query parameters.
- Clicking a day or task drills into the existing Daily View.
- Completion and reopen actions remain owned by the Daily View.
- Weekly and Monthly views use one inclusive SQLite range query and perform
  grouping in the service layer; avoid per-cell database queries.
- Dates beyond the 60-day generated horizon show a notice rather than implying
  that an empty future date has no schedule.
- Phase 4 adds no schema migration or frontend dependency.

## Phase 4 implementation notes

- `/weekly` and `/monthly` are implemented alongside the existing `/daily`
  view, with shared schedule-view navigation.
- `ScheduledInstance` is the common joined chore, instance, and assignee type;
  `DailyInstance` remains an alias for compatibility with the Daily View.
- `ListScheduledInstances` is the inclusive bounded-range store query used by
  both calendar views. It retains instances for inactive chores so historical
  calendar state remains visible.
- Weekly aggregation creates exactly seven household-local calendar days and
  is tested across year boundaries and daylight-saving transitions.
- Monthly aggregation always creates a Monday-first 42-day grid and is tested
  for leap-year and adjacent-month behavior.
- Weekly and Monthly handlers validate query parameters and render full templ
  pages. Empty household, empty schedule, and beyond-horizon states are
  represented explicitly.
- Weekly desktop and mobile layouts are distinct: the desktop uses the
  people-by-day grid, while small screens use day cards grouped by person.
- Monthly day cells link to the Daily View and summarize count, completion,
  overdue state, and per-person status without adding completion controls.
- Router, handler, service, SQLite, and application integration coverage exists
  for both views.
- The container image has been verified with both routes. A developer must
  ensure the configured host port is not already occupied by an older local
  `kinops` process when performing that verification.

## Phase 5 scope

Implement read-only, per-person subscribable ICS calendar feeds.

### Feed contract

- Expose `GET /calendar/{person-token}.ics`; do not expose numeric person IDs in
  feed URLs.
- Resolve only active people by their existing unique calendar token. Return a
  generic `404 Not Found` for an unknown or inactive token.
- Emit one all-day `VEVENT` for each persisted instance assigned to that person
  from the household-local current date through the generated 60-day horizon.
- Use persisted instances and assignments as the source of truth. The feed must
  not recalculate recurrence or rotation.
- Include pending, done, and skipped instances in the date range so marking an
  occurrence complete does not unexpectedly remove it from a subscribed
  calendar. Phase 5 does not encode completion status into event titles.
- Use a stable UID derived from the chore-instance ID and the application-owned
  `@kinops.local` namespace. Regeneration must not change the UID of a
  preserved instance.
- Use `DTSTART;VALUE=DATE` and exclusive next-day `DTEND;VALUE=DATE`. The schema
  has no due-time field, so timed calendar events are outside Phase 5.
- Include the chore name as `SUMMARY`; include description and category only
  when present. Do not put the calendar token or internal database path into
  feed content.
- Return `text/calendar; charset=utf-8`, an inline `.ics` filename, and a short
  private cache policy suitable for polling calendar clients.

### ICS encoding

- Generate RFC 5545-compatible `VCALENDAR` output with `VERSION:2.0`,
  `PRODID`, `CALSCALE:GREGORIAN`, and `METHOD:PUBLISH`.
- Use CRLF line endings, escape text values, and fold content lines by UTF-8
  octets at the 75-octet limit without splitting a code point.
- Format all-day dates as `YYYYMMDD`. Do not emit `VTIMEZONE` for date-only
  events.
- Keep events in deterministic due-date and instance-ID order so responses and
  tests are stable.
- An active person with no upcoming instances receives a valid empty calendar,
  not a `404`.

### Service and persistence

- Add store operations to resolve an active person by calendar token and list
  that person's joined scheduled instances for an inclusive date range.
- Implement the SQLite range query with the existing
  `chore_instances_person_due_date_idx`; do not fetch all household instances
  and filter them in Go.
- Add a small calendar-feed service that determines the household-local range
  and maps persisted data into a renderer-friendly feed model.
- Keep RFC 5545 serialization in a focused internal package, separate from HTTP
  handling and SQL.
- Phase 5 requires no migration, background worker, templ view, htmx behavior,
  OAuth flow, or third-party ICS dependency.

### Error handling and security

- Treat the token as a bearer secret: never log it, include it in an error
  message, or render it elsewhere in the application.
- Calendar handlers remain read-only and must not mutate the scheduling
  horizon. Horizon maintenance remains owned by application startup and the
  existing maintenance loop.
- Return `500 Internal Server Error` for store or rendering failures while
  logging only safe request context.

### Phase 5 tests

- Unit-test escaping, CRLF output, UTF-8-safe line folding, all-day end dates,
  stable UIDs, deterministic ordering, optional fields, and empty calendars.
- Store integration tests cover token lookup, inactive and unknown people,
  inclusive date bounds, assignment filtering, all statuses, and ordering.
- Service tests pin the household-local current date and verify the 60-day
  horizon without DST date drift.
- Handler tests cover content headers, valid and empty feeds, malformed or
  unknown tokens, inactive people, and internal errors without token leakage.
- Router/application integration tests create multiple people with assignments
  and prove that each token's feed contains only that person's occurrences.
- Run `just check`, build the container, and request a feed from the running
  image before Phase 5 is complete.

### Recommended Phase 5 commit sequence

1. Add the calendar-feed domain model and person-scoped store interface.
2. Implement and integration-test SQLite token lookup and instance queries.
3. Add and unit-test the dependency-free RFC 5545 serializer.
4. Add the feed service and household-timezone range tests.
5. Add the HTTP handler, route, safe headers, and handler tests.
6. Add end-to-end per-person isolation coverage and container verification.

## Phase 5 implementation notes

- `GET /calendar/{person-token}.ics` is implemented as a read-only route with
  generic not-found behavior for malformed, unknown, or inactive tokens.
- Feed requests resolve the active person by token and use an indexed,
  person-scoped inclusive instance query; household instances are never loaded
  and filtered in Go.
- The service includes persisted pending, done, and skipped occurrences from
  the household-local current date through day 60 and does not recalculate
  assignment or recurrence.
- `internal/ical` provides dependency-free RFC 5545 serialization with CRLF
  endings, text escaping, deterministic ordering, stable instance UIDs,
  exclusive all-day end dates, and UTF-8-safe 75-octet line folding.
- The handler buffers output before writing headers, returns a private
  five-minute cache policy, and never logs the token or token-bearing path.
- Serializer, store, service, handler, router, and full application tests cover
  empty feeds, token safety, DST-safe bounds, all statuses, deterministic
  encoding, and strict per-person isolation.
- Phase 5 adds no migration, templ view, htmx behavior, background worker, or
  third-party dependency.

## Remaining MVP phases

- Phase 7: Final styling, empty-state, accessibility, and mobile-responsive
  polish.

## Family hub expansion (in progress)

KinOps is expanding from chore-only views into a combined household agenda.
The first implemented vertical slice adds locally managed events to the Daily
View while preserving the existing one-tap chore workflow.

- `CalendarFeedEvent` is the outbound chore ICS DTO. Do not reuse it for local
  or imported household events.
- `HouseholdEvent` stores the editable event definition;
  `EventOccurrence` stores materialized dates/times used by views.
- Local event recurrence supports one-off, daily, every N days, specific
  weekdays, monthly day-of-month, and annual month/day.
- Annual February 29 occurrences are skipped in non-leap years. Invalid
  monthly dates are skipped rather than clamped.
- Event definitions use household-local dates and wall-clock times. Timed
  occurrences are materialized as UTC instants so recurring events retain
  their intended local time across daylight-saving transitions.
- All-day end dates are exclusive. Timed event end dates are the actual local
  ending date.
- An event with no audience rows is household-wide. One or both active people
  may instead be selected explicitly.
- Local event occurrences are generated 400 days forward from the
  household-local current date. Chore instances retain their existing 60-day
  horizon.
- Editing an event preserves occurrences before today and regenerates
  occurrences starting today. Archiving removes occurrences starting today or
  later.
- Local event CRUD is a public household feature under `/events`, matching the
  existing LAN-only chore management model. The protected admin area remains
  reserved for analytics and calendar-feed configuration.
- Event categories are a curated list defined in the domain layer and rendered
  as a dropdown. Adding or renaming a category is a code/deployment change, not
  a schema migration. Existing non-list values remain available while editing
  so older data is not silently discarded.
- The Daily View lists all-day events before timed events and keeps chore
  completion controls unchanged.
- The Monthly View shows up to two labeled event chips per day alongside the
  existing chore completion ratio and person dots. Multi-day occurrences are
  shown on every overlapping day and counted once in the month summary.
- The desktop Weekly View shows an Events row above the person-assignment rows.
  Mobile day cards show their events before person-grouped chores. Multi-day
  occurrences appear on each overlapping day and count once in the summary.

Not yet implemented in this expansion: external read-only ICS subscriptions,
imported-event exception handling,
combined outbound feeds, or the kitchen display route.

## Phase 6 and admin scope

Implement a session-protected administration area that owns analytics and
calendar-feed management.

- `/admin/login` provides the only admin login form.
- `/admin` and all admin mutations require an authenticated server-side
  session. Public chore views and ICS subscription routes remain unchanged.
- Admin credentials come from `KINOPS_ADMIN_USERNAME` and
  `KINOPS_ADMIN_PASSWORD_HASH`. The password hash uses PBKDF2-SHA256 with a
  random salt; plaintext passwords are never stored in SQLite or source code.
- Admin routes are disabled when both credential settings are absent. Providing
  only one credential setting is a configuration error.
- Session IDs are cryptographically random, stored only as hashes in memory,
  expire after 12 hours, and are carried by an HttpOnly, SameSite=Strict cookie
  scoped to `/admin`. Deployments using HTTPS can enable the Secure flag.
- Login rotates the session ID. Logout invalidates it immediately.
- State-changing admin requests require the session's CSRF token.
- Login errors are generic and neither passwords, session IDs, CSRF tokens, nor
  calendar tokens may be logged.

The admin dashboard includes:

- household completion rates for the inclusive last 7 and 30 calendar days;
- per-person assigned and completed counts for the last 30 days;
- a current per-person streak of fully completed chore-days, skipping days on
  which that person had no assigned chores;
- pending overdue counts, including occurrences older than 30 days;
- recent append-only completion activity;
- the configured household timezone and 60-day feed horizon;
- protected, copyable per-person ICS subscription URLs; and
- confirmed per-person calendar-token rotation, which invalidates that
  person's previous subscription URL.

Completion-rate denominators include every persisted instance due in the
window. Only `done` counts as completed; pending and skipped instances remain
in the assigned total. Analytics use persisted instances and logs rather than
recalculating schedules.

Phase 6 adds no migration or charting dependency. Use compact server-rendered
metrics and CSS progress bars; keep aggregation in the store and service
layers, and protect against per-card database queries.

## Working style

Before editing:

1. Inspect the relevant code.
2. Explain the proposed design.
3. List files to change.
4. Identify tests to add.
5. Wait for approval unless explicitly asked to implement.

Keep changes small and reviewable.

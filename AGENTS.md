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
- KinOps and its Mealie integration dependency are containerized with Docker
  Compose
- No CGO
- No frontend build toolchain

## MVP direction

Build the simplest reliable household operations application that looks good
on a desktop or phone and runs on a trusted local network. KinOps remains one
Go binary; Mealie runs as a separate service because it is the source of truth
for recipes, meal plans, and shopping lists.

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

## Phase 7 scope

Complete the MVP styling, empty-state, accessibility, and mobile-responsive
polish without adding a frontend build system or changing application
behavior.

- Keep dark mode as the default and only MVP appearance.
- Use one responsive primary navigation that remains keyboard-accessible and
  works without Alpine or htmx.
- Preserve at least 44 CSS-pixel touch targets for primary controls and chore
  completion actions.
- Stack page headers, action groups, forms, cards, Kitchen controls, and admin
  controls cleanly on phone-width screens.
- Keep Weekly's existing day-card mobile presentation and make Monthly remain
  a seven-column touch calendar with reduced, glanceable density.
- Provide consistent focus-visible treatment, a skip link, reduced-motion
  support, loading announcements, readable empty states, and sufficient dark
  palette contrast.
- Phase 7 adds no schema migration, backend route, JavaScript dependency, or
  external font/image dependency.

### Phase 7 implementation notes (completed 2026-08-02)

- The sticky shared header now has KinOps branding and a native
  `details`/`summary` phone menu. Desktop navigation remains visible, while
  the phone menu requires no client-side framework and exposes large stacked
  links.
- The base layout includes a keyboard skip link, a programmatically focusable
  main landmark, an announced htmx loading state, consistent focus-visible
  rings, and reduced-motion behavior.
- Shared typography, spacing, card hierarchy, hover treatment, form controls,
  buttons, and empty states use the existing dependency-free dark palette.
  The home and empty chore views now offer clear next actions.
- Phone layouts use full-width date navigation, wrapped or stacked page
  actions, single-column cards and forms, compact chore completion buttons,
  touch-sized calendar tasks, single-column Kitchen recipe/grocery controls,
  and stacked event/admin controls.
- Weekly continues to replace its desktop people-by-day grid with day cards
  below tablet width. Monthly retains the familiar seven-column grid but uses
  event dots, compact completion ratios, smaller status dots, and larger day
  hit areas below 560 pixels.
- Monthly highlights the household-local current date with a visible Today
  badge and inset glow. Past dates have a separate touch-sized cross-out
  control whose state is stored only in that browser; it does not mutate chore
  completion, event data, or another device's calendar.
- A layout rendering test protects the skip link, native responsive menu,
  main focus target, and live loading announcement. `just check` passes with
  templ generation, vet, and race-enabled tests.

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
- Event color follows audience across the Events, Daily, Weekly, and Monthly
  views. A single-person event uses that person's configured color; household
  and multi-person events use the configurable household event color stored in
  the singleton `household_settings` row and edited on the People page.

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

## Kitchen and Mealie integration roadmap

Kitchen is a separate KinOps product area under `/kitchen`. Its Daily and
Weekly views show meals only; they do not mix in chores or household events.
It also contains recipe search/favorites and grocery-list interactions. A
future Display area will compose chores, events, and meals for the wall-mounted
kitchen screen, but Display is not part of the Kitchen implementation phases.

### Mealie ownership boundary

- Mealie is the only source of truth for recipes, favorites, meal plans,
  shopping lists, and shopping-list items.
- Do not add KinOps SQLite tables for Mealie resources and do not copy or sync
  them into the KinOps database.
- KinOps may keep a short-lived in-memory read cache, but cached data is never
  authoritative and is discarded on process restart.
- Do not implement recipe scraping, parsing, ingredient normalization,
  serving conversion, grocery deduplication, or recipe editing in KinOps.
- Keep Mealie wire DTOs inside `internal/mealie`; handlers and templates use
  smaller KinOps domain/view types assembled by `internal/kitchen`.
- Treat every Mealie identifier as an opaque string.
- The Mealie token is server-only. Never render it, put it in browser-visible
  state, include it in a URL, or log it or an Authorization header.

### Pinned development instance and verified API contract

- Docker Compose runs `ghcr.io/mealie-recipes/mealie:v3.22.0` as service
  `mealie`, publishes it on host port `9925`, and persists it in the
  `mealie-data` volume.
- KinOps will call Mealie over the Compose network at
  `http://mealie:9000`. Browser-facing links use `MEALIE_PUBLIC_URL`, normally
  `http://localhost:9925` in local development.
- Mealie's health probe is `GET /api/app/about`; its live OpenAPI document is
  available at `/openapi.json` and interactive documentation at `/docs`.
- The v3.22.0 OpenAPI document was inspected on 2026-08-01. The following
  authenticated reads were also exercised against the running container and
  returned `200 OK`:
  - `GET /api/recipes`;
  - `GET /api/households/mealplans`;
  - `GET /api/households/shopping/lists`;
  - `GET /api/households/shopping/items`; and
  - `GET /api/users/self/favorites`.
- The verified mutation contract is:
  - `POST /api/households/mealplans` with `CreatePlanEntry`;
  - `PUT` and `DELETE /api/households/mealplans/{item_id}`;
  - `POST /api/households/shopping/items` with `ShoppingListItemCreate`; and
  - `PUT` and `DELETE /api/households/shopping/items/{item_id}`.
- Recipe search uses the `search`, `page`, and `perPage` query parameters on
  `GET /api/recipes`. Favorites use `/api/users/self/favorites` for reads and
  `/api/users/{id}/favorites/{slug}` for add/remove operations.
- Before implementing the client against another Mealie version, compare its
  live `/openapi.json` with this contract and update fixtures deliberately.

### Required local operator setup

- Log into `http://localhost:9925` on a fresh instance using Mealie's
  documented bootstrap credentials, immediately change the password, and keep
  public signup disabled.
- Create a dedicated long-lived API token from
  `/user/profile/api-tokens`. Store it only in ignored local configuration as
  `MEALIE_API_TOKEN`.
- Configure `MEALIE_BASE_URL=http://mealie:9000` and
  `MEALIE_PUBLIC_URL=http://localhost:9925` for local Compose development.
- Add representative recipes, favorites, meal-plan entries, at least one
  shopping list, active and checked shopping items, labels, and a meal-plan
  note. These records become the manual smoke-test dataset; do not commit their
  database or token.

### Kitchen Phase K1 — Configuration and typed client foundation

Add optional Mealie configuration and the dependency-free HTTP client.

- Add `MEALIE_BASE_URL`, `MEALIE_PUBLIC_URL`, `MEALIE_API_TOKEN`,
  `MEALIE_DEFAULT_SHOPPING_LIST_ID`, and `MEALIE_REQUEST_TIMEOUT`.
- Base URL and token must be supplied together. When both are absent, KinOps
  starts normally and Kitchen displays a configuration state.
- Validate URL schemes, trim trailing slashes, and use one shared
  `http.Client` with a short timeout, connection reuse, bounded response
  bodies, and a KinOps User-Agent.
- Add typed errors for unauthorized, forbidden, not found, validation,
  rate-limited, unavailable, malformed-response, and unexpected-status cases.
- Decode Mealie pagination explicitly; never fetch unbounded result sets.
- Add private transport DTOs for only the verified recipe, favorite,
  meal-plan, shopping-list, and shopping-item fields KinOps needs.
- Tests use `httptest.Server` and prove bearer authentication, token
  redaction, pagination/query encoding, response limits, cancellation, error
  mapping, and representative v3.22.0 fixtures.
- Exit criterion: the client can perform every required read against a fake
  server and a manual smoke command can read the local Mealie instance.

### Kitchen Phase K1 implementation notes

- K1 was completed against the pinned Mealie v3.22.0 development instance.
- `internal/config` accepts the optional Mealie base/public URLs, API token,
  default shopping-list ID, and bounded request timeout. Base URL and token are
  an all-or-nothing pair; absent integration configuration does not prevent
  KinOps from starting.
- `internal/mealie` owns typed, bounded reads for application metadata,
  recipes, meal plans, shopping lists, shopping items, and favorites. It uses
  one injected HTTP client, server-only bearer authentication, explicit
  pagination, safe typed errors, response-size limits, and cancellation.
- Unit tests cover the verified v3.22.0 response shapes, query encoding,
  authentication, pagination validation, response limits, malformed data,
  cancellation, upstream status mapping, and token/body redaction.
- `just mealie-smoke` is an opt-in real-instance contract test. It overrides
  only the host-side base URL, reads the ignored `.env` token, and reports
  aggregate counts without printing resource data or credentials.
- The first real smoke read found one recipe, one meal-plan entry, one shopping
  list, and ten shopping items. Favorites were empty and decoded successfully.

### Kitchen Phase K2 — Kitchen shell and read-only meal views

Create the separate Kitchen area and read-only Daily/Weekly meal planning
views.

- Add a top-level Kitchen navigation link and a dedicated Kitchen tab bar for
  Daily, Weekly, Recipes, and Groceries.
- Add `GET /kitchen`, `/kitchen/daily?date=YYYY-MM-DD`, and
  `/kitchen/weekly?date=YYYY-MM-DD`; `/kitchen` redirects to Daily.
- Kitchen dates use the KinOps household timezone. Weeks remain Monday through
  Sunday.
- Define narrow domain/view types such as `RecipeSummary`, `MealPlanEntry`,
  `KitchenDay`, and `KitchenWeek`; do not expose Mealie DTOs to templ.
- Daily groups entries by Mealie plan-entry type. Weekly uses seven desktop
  columns and stacked mobile day cards.
- Support recipe-backed entries and Mealie note entries. Preserve multiple
  entries in the same date/type slot.
- Render explicit integration-disabled, empty, loading, unauthorized, and
  unavailable states. A Mealie failure must not affect chores/events routes or
  `/healthz`.
- Exit criterion: representative plans created in Mealie appear on the correct
  household-local day in both views, including a week boundary.

#### K2 implementation notes (completed 2026-08-02)

- `/kitchen` now redirects to `/kitchen/daily`. Daily and Weekly are live
  server-rendered views; Recipes and Groceries are visible disabled tabs until
  their implementation phases.
- `internal/kitchen.Service` is the boundary between Mealie transport DTOs and
  the narrow Kitchen view types in `internal/domain`. It performs bounded
  pagination, parses date-only values in the household location, creates
  Monday-through-Sunday weeks, groups meal slots in a stable order, and keeps
  multiple entries in the same slot.
- Recipe-backed and note-only plan entries render without requiring Mealie's
  recipe editor. Browser links into Mealie are intentionally deferred to K3 so
  they can use verified group metadata instead of a guessed frontend path.
- A disabled integration renders setup guidance. Invalid or forbidden tokens
  and other upstream failures render distinct degraded Kitchen pages with HTTP
  200, while malformed user dates remain HTTP 400. These paths do not affect
  chores, events, or `/healthz`.
- Kitchen navigation uses the existing htmx-boosted main region and now exposes
  an explicit loading indicator. The weekly grid uses seven columns on wider
  screens and stacked day cards below 900 px.
- Fake-client service tests cover household-local dates, Monday boundaries,
  pagination, grouping, multiple same-slot entries, and recipe mapping.
  Handler/router tests cover enabled, disabled, unauthorized, unavailable, and
  invalid-date behavior.
- `just check` passes. A real Mealie v3.22.0 dinner dated 2026-08-03 was
  rendered on that date in Daily and in the 2026-08-03 through 2026-08-09
  Weekly view.

### Kitchen Phase K3 — Meal scheduling and recipe discovery

Add recipe browsing, favorites, and meal-plan mutations.

- Add `GET /kitchen/recipes` with debounced htmx search, pagination, and a
  favorite-only filter.
- Recipe cards show available name, image, categories, rating/favorite state,
  and preparation/total time without trying to reproduce Mealie's editor.
- Add an authenticated, allowlisted KinOps image proxy only if Mealie images
  cannot be loaded safely without exposing the API token.
- Add `POST /kitchen/meals`, `PUT /kitchen/meals/{id}`, and
  `DELETE /kitchen/meals/{id}` as narrow server-side adapters to Mealie.
  POST form fallbacks with `_method` remain available for browser robustness.
- Recipe selection must preserve the clicked date and meal-entry type from
  Daily or Weekly. Allow scheduling a recipe or a note.
- Provide “Open in Mealie” links using `MEALIE_PUBLIC_URL`; never use the
  container-only base URL in browser HTML.
- Mutations return focused htmx fragments, invalidate affected read caches,
  and change the UI only after Mealie confirms success.
- Exit criterion: a user can search/favorite a real recipe, schedule it for a
  date, see it in both Kitchen views, edit it, and remove it.

#### K3 implementation notes (completed 2026-08-02)

- `/kitchen/recipes` supports server-rendered and focused htmx search,
  300-millisecond debouncing, API-backed pagination, and a favorite-only
  filter. Favorite-only results are filtered before KinOps pagination so page
  totals remain accurate.
- Recipe cards expose available image, category, rating, preparation/total
  time, favorite state, and a group-aware “Open in Mealie” link. The verified
  v3.22 media endpoint serves browser-safe recipe images without
  authentication, so no image proxy or browser-visible token was added.
- `GET /api/users/self` supplies the server-side user ID and group slug needed
  by favorites and browser links. Meal updates first read the existing Mealie
  entry and reuse its server-owned `groupId` and `userId`; those ownership
  fields are never accepted from the browser.
- `POST /kitchen/meals`, `PUT /kitchen/meals/{id}`, and
  `DELETE /kitchen/meals/{id}` are live, along with POST `_method` fallbacks.
  Users can schedule either a recipe or a note, then edit date, meal slot, and
  note fields or remove the entry.
- Daily and Weekly add links preserve the selected household-local date.
  Existing slot links also preserve the precise meal-entry type. The recipe
  page keeps both values across search, favorite, and pagination interactions.
- Recipe scheduling and favorite controls return focused htmx fragments.
  Meal edits replace only the confirmed card, deletes remove only the
  confirmed card, and failed upstream writes append an inline error without
  optimistically changing the visible meal.
- Client tests cover verified create/read/update/delete meal payloads,
  authenticated favorite mutations, status mapping, and response redaction.
  Service/handler/router tests cover recipe mapping, images and group links,
  ownership-safe updates, fragments, fallbacks, and disabled states.
- `just check` passes. Real Mealie v3.22.0 acceptance verified recipe search,
  image loading, group-aware links, favorite add/remove, recipe scheduling,
  Daily/Weekly visibility, moving the entry to a new date/type, and deletion.
  The temporary entry was removed and the tested recipe was restored to its
  original unfavorited state.

### Kitchen Phase K4 — Grocery lists and touch interactions

Add shopping-list selection and high-frequency grocery operations.

- Add `GET /kitchen/groceries?list={id}`. If a configured default exists, use
  it; otherwise auto-select only when Mealie returns exactly one list and show
  a selector when several exist.
- Define `ShoppingList` and `ShoppingItem` view types. Preserve Mealie label
  and sort information where returned.
- Show active items grouped by label with large touch targets; checked items
  appear in a collapsed section and can be reopened.
- Add `POST /kitchen/groceries/items`, plus `PUT` and `DELETE` routes for an
  individual item. KinOps PATCH-style htmx requests may be accepted locally,
  but the Mealie adapter uses its verified PUT contract.
- Support quick-add, check/uncheck, quantity/unit changes supported by the API,
  and item deletion. Advanced label ordering and recipe ingredient import stay
  in Mealie via browser links.
- Do not optimistically remove or check an item before the upstream write
  succeeds. Upstream validation errors render beside the initiating control.
- Exit criterion: add, check, uncheck, edit, and remove a real item, with the
  result visible in both KinOps and Mealie.

#### K4 implementation notes (completed 2026-08-02)

- `/kitchen/groceries?list={id}` is live. A valid configured default is used
  first, a sole list is auto-selected, and multiple lists without a default
  require an explicit user choice. Missing configured defaults remain visible
  as a recoverable selector state.
- Shopping-list and item DTOs are mapped into narrow Kitchen view types.
  Active items retain Mealie label names, colors, label order, and item
  positions. Checked items remain in a collapsed section with a large reopen
  control instead of disappearing.
- Quick-add resolves foods and optional units by exact case-insensitive name
  before writing the item. Existing ingredient IDs are reused; a new Mealie
  ingredient is created only when no exact match exists. This matches Mealie's
  v3.22 workflow and avoids duplicate foods for common entries.
- Item updates first read the authoritative Mealie row, then preserve its
  shopping-list, food, label, position, and unchanged unit identifiers.
  Quantity, unit, note, display, and checked state are the only browser-edited
  values.
- `POST /kitchen/groceries/items`, `PUT`, `PATCH`, and `DELETE` item routes are
  live with POST `_method` fallbacks. The adapter always uses Mealie's verified
  single-item PUT contract. DELETE carries list context in its query because
  Go intentionally does not parse URL-encoded DELETE bodies.
- Successful htmx writes refresh the focused grocery-list fragment only after
  Mealie confirms the change. Failed writes retain the current item and render
  beside the initiating add/edit control; KinOps never pretends an upstream
  write succeeded.
- Touch controls use three-inch-equivalent targets, responsive single-column
  edit forms, inline quantities, and collapsible checked items for kitchen
  display use.
- Client tests cover food/unit lookup and creation, collection-shaped item
  responses, and item GET/POST/PUT/DELETE. Service/handler/router tests cover
  list selection/defaults, grouping, ownership-field preservation, fragments,
  inline errors, DELETE list preservation, and disabled states.
- `just check` passes. Real Mealie v3.22.0 acceptance verified two-list
  selection, exact food-ID reuse, add, quantity/unit/note edit, check, reopen,
  and delete. Temporary malformed contract-probing rows and the final test item
  were removed; no existing shopping item was modified.

### Kitchen Phase K5 — Resilience, caching, and deployment verification

Harden the completed Kitchen area without turning it into a synchronization
system.

- Add a small bounded in-memory cache for recipe pages and recent meal-plan
  reads. Grocery writes and meal writes invalidate related entries.
- Permit clearly labeled stale reads only when a prior value exists; never
  queue or pretend to complete a failed write.
- Add safe upstream observability: operation name, duration, status class, and
  request ID only. Never log tokens, Authorization headers, or arbitrary
  upstream bodies.
- Show Mealie version/connectivity on the protected admin dashboard, but do not
  make Mealie part of KinOps' core health check.
- Test timeouts, restarts, stale fallback, cache invalidation, malformed JSON,
  `401`, `403`, `404`, `422`, `429`, and `5xx` responses.
- Run `just check`, build both ARM64 containers, start the full Compose stack,
  and complete the real-data recipe/plan/grocery smoke path.
- Exit criterion: restarting or stopping Mealie leaves chores/events usable,
  and Kitchen recovers without restarting KinOps when Mealie returns.

#### K5 implementation notes (completed 2026-08-02)

- Kitchen read caching is process-local, bounded to 64 least-recently-used
  entries per resource, and remains a performance/resilience layer rather
  than a second source of truth. Meal-plan ranges are fresh for 30 seconds,
  recipe queries for two minutes, and selected grocery views for 15 seconds;
  prior values may be used as explicitly labeled stale snapshots for up to 24
  hours only after an upstream refresh fails.
- Successful meal mutations clear meal-plan range entries, successful
  favorite changes clear recipe queries, and successful grocery mutations
  clear selected-list entries. Failed writes do not invalidate, queue,
  optimistically apply, or otherwise pretend to persist a change.
- Daily, Weekly, Recipes, and Groceries render a visible “Mealie connection
  interrupted” warning whenever a stale snapshot is served. With no prior
  snapshot, existing unauthorized/unavailable states remain authoritative.
- The Mealie transport emits structured request telemetry containing only a
  stable operation name, elapsed duration, HTTP status class, and chi request
  ID. It does not log request URLs, identifiers, tokens, headers, payloads, or
  arbitrary upstream response bodies.
- The protected admin dashboard probes `/api/app/about` and displays enabled,
  connected, version, and checked-at state. This optional probe is separate
  from the SQLite-backed KinOps `/healthz` endpoint.
- Tests cover bounded eviction, fresh reads, stale fallback, stale expiry,
  recovery, meal/favorite/grocery invalidation, admin status rendering,
  timeout/cancellation, malformed and oversized JSON, safe telemetry, and
  `401`, `403`, `404`, `422`, `429`, and `5xx` mappings. `just check` passes
  with the race detector.
- Both local images were verified as ARM64. The full Compose stack rendered
  real recipe, meal-plan, and shopping-list data. Stopping only Mealie left
  `/healthz`, chores, and events available; cached meal and grocery pages
  showed stale warnings after their TTLs. Starting only Mealie restored fresh
  Kitchen reads without restarting KinOps.

### Future Display phases — not part of Kitchen delivery

- Display D1: add a read-only `MealsBetween` Kitchen service boundary and a
  composition service that combines persisted chores, household events, and
  Mealie meals without copying data between stores.
- Display D2: implement `/display/daily` and `/display/weekly` for a 2560x1440
  touch display with large typography, glanceable status, and restrained
  navigation.
- Display D3: implement `/display/monthly` with household availability/events,
  chore density/completion, and meal indicators rather than full detail in
  every cell.
- Display D4: add kiosk concerns including automatic refresh, network/offline
  state, wake/sleep behavior, burn-in-aware movement, Raspberry Pi Chromium
  launch, and touch-only recovery controls.

### Recommended Kitchen commit sequence

1. Pin and run Mealie; document the verified v3.22.0 API contract.
2. Add optional configuration and the typed Mealie client skeleton.
3. Add client fixtures, pagination, error mapping, and authentication tests.
4. Add the Kitchen navigation shell and read-only Daily view.
5. Add the read-only Weekly view and timezone-boundary tests.
6. Add recipe search, favorites, pagination, and safe images/links.
7. Add meal-plan create, update, and delete interactions.
8. Add shopping-list selection and read-only grouping.
9. Add grocery item create, check/uncheck, edit, and delete interactions.
10. Add caching, degraded states, safe observability, and admin connectivity.
11. Complete full-stack, ARM64, and Raspberry Pi smoke verification.

## Working style

Before editing:

1. Inspect the relevant code.
2. Explain the proposed design.
3. List files to change.
4. Identify tests to add.
5. Wait for approval unless explicitly asked to implement.

Keep changes small and reviewable.

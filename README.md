# DynaR

**DynaR** is a standalone, read-only Go CLI for browsing all Dynatrace Grail logs in a Kubernetes namespace. **Its interactive picker and log viewer now use tailg's actual shared engine**, not a parallel imitation. The DynaR binary and releases remain independent. It is separate from [tailg](https://github.com/fernandocpaz/tailg): it does **not** require `kubectl` or a Kubernetes connection.

## Quick start

Install Go 1.25 or later:

```bash
go install github.com/fernandocpaz/dynar/cmd/dynar@latest
dynar source add prod --url https://YOUR-TENANT.apps.dynatrace.com --default
export DYNAR_PROD_TOKEN='YOUR_DYNATRACE_PLATFORM_TOKEN'
dynar -n apollo
```

On PowerShell, use `$env:DYNAR_PROD_TOKEN = 'YOUR_DYNATRACE_PLATFORM_TOKEN'` instead of `export`.

## Updates (same workflow as tailg)

DynaR checks GitHub's latest stable release once at interactive startup, without delaying namespace log collection. When a newer stable version is published, an in-terminal prompt offers **Later** (the default) or **Update & restart**. Press `U` to install immediately, `Tab` to change the focused action, `Enter` to select, or `Esc` to continue. DynaR will verify the downloaded executable's SHA-256 digest, check the version it reports, safely replace the installed binary, and restart with the same arguments. A failed update preserves the existing executable.

```bash
dynar update --check     # compare installed vs latest stable release
dynar update             # install verified release and exit
dynar -n apollo --no-update-check   # disable automatic startup check
```

Set `DYNAR_NO_UPDATE_CHECK=1` to disable checks by default. Updates require a writable install directory. Standalone snapshots and redirected output never prompt. Development builds do not offer automatic updates. Published releases include binaries for Linux, Windows, and macOS where supported; existing source credentials/configuration are not modified.

## Terminal UI: one panel per service

In an interactive terminal, `dynar -n apollo` opens tailg's workload picker, then tailg's real full-screen log viewer for the selected workload(s). Select several with Space; use Enter to open.  It loads all logs using one namespace-wide Grail query, then automatically groups by `k8s.workload.name`, deployment, StatefulSet/DaemonSet/Job, container, or inferred pod workload. The shared log viewer formats Serilog/JSON, colored ERR/WRN/INF severity, timestamps, and multiline exceptions using tailg's production renderer. Terminal widths determine whether panels appear in one, two or three columns. Tab selects a workload; Enter maximizes it for scrolling.

| Key | Behavior |
| --- | --- |
| Tab / Shift+Tab | Select next / previous service |
| Enter / Esc | Maximize service / return to grid |
| A | Combined timeline across all services |
| / | Search across all services and trace IDs |
| F1 | Toggle only matching messages |
| F3 | Toggle errors only |
| C | Clear search |
| Up/Down / PgUp/PgDn | Scroll in maximized view; select or paginate in grid |
| Space / R | Pause/resume live polling / refresh now |
| Q / ? | Quit / keyboard help |

`--search` initially restricts displayed messages in UI; F1 toggles between all messages (matches highlighted) and matching-only. Use `--no-ui` for traditional streaming, or `--once --group` to print a static service-by-service report. Redirected output and NDJSON always use noninteractive mode, preserving automation.

The terminal will never silently claim full log coverage: query limits, sampling, permissions, and missing namespace attributes still apply. The UI shows the latest query warning.

The token must have permission to query Grail logs, including `storage:logs:read` and applicable bucket read permissions. A Dynatrace **platform token** or OAuth2 access token works with the Platform Query API; classic `Api-Token` credentials do not. Keep tokens out of source control and command-line arguments. An alternative is OAuth client credentials, configured using `DYNAR_PROD_CLIENT_ID` and `DYNAR_PROD_CLIENT_SECRET`.

## Primary commands

```bash
dynar -n apollo                             # tailg workload picker → tailg live log viewer
dynar -n apollo --source qa --cluster tkgs-qa
dynar -n apollo --since 24h --once          # historical logs once
dynar -n apollo --search 'timeout'          # text search across all services
dynar -n apollo --level ERROR --service encounter
dynar -n apollo --search 'timeout|failed' --regex
dynar -n apollo --once --output ndjson > logs.ndjson
dynar -n apollo --once --group                # snapshot grouped by service
dynar -n apollo --no-ui                       # legacy plain streaming mode
dynar -n apollo --all                         # open all services in a shared timeline

dynar source list
dynar source use qa
```

`dynar -n apollo` executes `fetch logs | filter k8s.namespace.name == "apollo"`, including log records for **every service, pod and container with that namespace attribute**. No application discovery or hardcoded service lists are needed. Default lookback: **30 minutes**. Default refresh: **10 seconds**. Override these with `--since` and `--refresh`.

## Data retrieval and reliability

- Runs read-only queries through Dynatrace Grail Query API (`query:execute` and `query:poll`).
- Saturated queries are recursively split into smaller time windows until the budget is exhausted (`--max-queries`, default 32).
- Prints explicit warnings when query limits or sampling prevent complete results.
- Keeps a bounded memory buffer (`--buffer-lines`, default 20,000), sorts chronologically, and deduplicates known records across a five-minute polling overlap.
- Offers interactive per-service panels, plain text and NDJSON output. CLI search/filter uses `--search`, `--regex`, `--service`, and `--level`. Full-screen service tiles, inline search, and a combined timeline are included; charts are planned.
- Namespace logs must be tagged with `k8s.namespace.name`. The CLI cannot retrieve logs that Dynatrace did not ingest, records beyond retention, or records blocked by IAM restrictions.

Grail Query API is intended for bounded queries, not bulk data export. Extremely large time ranges may be expensive or incomplete; DynaR warns where known. Refresh mode polls rather than using a server push/stream. Delayed ingestion beyond five minutes may require a historical query.

## Sources and credentials

`dynar source add NAME --url https://TENANT.apps.dynatrace.com` stores only the source URL in the OS user configuration directory (`dynar/config.json`). `--default` makes it the default source. `DYNAR_CONFIG` overrides the configuration file path. If no named sources are configured, `DYNAR_URL` can provide the base URL.

Per-source `DYNAR_<SOURCE>_TOKEN` and global `DYNAR_TOKEN` variables are supported. OAuth client credentials can be provided through `DYNAR_<SOURCE>_CLIENT_ID` and `DYNAR_<SOURCE>_CLIENT_SECRET` (or the global variables). Source names are uppercased with dashes replaced by underscores.

## Development

```bash
go test ./...
go vet ./...
go build -o dynar ./cmd/dynar
```

DynaR is an independent repository; tailg is unchanged.

References: [Dynatrace platform tokens](https://docs.dynatrace.com/docs/manage/identity-access-management/access-tokens-and-oauth-clients/platform-tokens) and [Grail Query API](https://developer.dynatrace.com/develop/platform-services/services/grail-service/).

## Shared UI and provider-only functionality

Interactive mode uses `github.com/fernandocpaz/tailg/pkg/logui`. DynaR queries Grail **once per namespace polling interval**, then supplies the results to tailg's UI via an in-memory multiplexer. Source and authentication remain Dynatrace; tailg remains fully usable with Kubernetes unchanged.

The exact tailg log-viewer shortcuts replace DynaR's former prototype grid shortcuts. In particular, F1 filters matches, F3 opens retained error logs, F5 shows heartbeat analysis, F8 issues, F9 analytics/charts, F10 security, and F6 traces. The workload picker uses Space multi-select and Enter open. `--all` skips the picker. Full-history searches use the selected `--since` window and fail visibly if query completeness cannot be confirmed.

Kubernetes-only picker actions (pod commands, CronJob execution, node diagnostics, ConfigMaps/Secrets, pod metrics, network inspection) are disabled in the Dynatrace provider. No `kubectl` is required. The original separate prototype UI remains in the codebase only for reference and is not invoked by default.

## Search and analysis parity

DynaR uses tailg's native filter/query parser for both live logs and historical
results. Type in the search bar to use expressions such as
`level:ERR service:encounter`, `trace:<32-char-id>` or `status:>=500`.
`dynar -n apollo --search "timeout"` initializes that filter. With
`--regex --search "timeout|failed"`, DynaR uses tailg's include-regex
formatter instead. `--since` controls the bounded historical lookup.

The F3 error inspector now retains stack frames split into multiple Grail
records using tailg's original exception continuation rules.

AI analysis is opt-in per selected log and uses the same provider configuration
and source-repository mapping already configured for tailg
(`TAILG_AI_*` and tailg's owner-restricted saved settings). No Dynatrace logs
are sent to an AI provider unless the operator explicitly invokes an AI action.

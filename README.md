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

## Terminal UI — the actual tailg picker and log viewer

Running `dynar -n apollo` queries all accessible logs in the namespace once,
then opens the **real tailg workload picker**. Use Up/Down to navigate, Space
to select multiple services, and Enter to open their log streams. The viewer
is tailg's production terminal implementation, not the old DynaR prototype
grid. Logs are formatted using tailg's original Serilog, JSON, date grouping,
severity colors, exception details and wrapping.

| Key | Behavior |
| --- | --- |
| Up/Down, Space, Enter | Picker navigation, multi-select, open selected workloads |
| Type in filter bar | Search live and retained history using tailg's exact query syntax |
| F1 | Toggle matches-only / surrounding context |
| F3 | Open full ERR/FTL history for the selected pod |
| F5 | Heartbeat analysis |
| F6 | Trace correlation |
| F8 | Issue radar |
| F9 | Traffic and latency analytics / charts |
| F10 | Security radar |
| S | Security assessment for the selected log |
| Enter | Open the highlighted full log detail |
| A (in detail) | AI explanation (if configured and explicitly invoked) |
| Esc | Close current detail/diagnostic view |
| Q / Ctrl+C | Leave viewer; return to picker or exit |

The picker F action for live-only recording is being brought across as a
separate adapter feature. `--all` intentionally skips the workload picker and
opens one combined timeline. `--no-ui`, `--once --group`, and redirected
NDJSON output remain available for automation.

In interactive mode, `--search` preloads a tailg filter; `--regex --search`
uses tailg's regular-expression include mechanism. `F1` changes the
match/context display. Only logs ingested and accessible in Dynatrace can be
displayed; retention, permissions, sampling and query budgets still apply.

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

Kubernetes-only picker actions (pod commands, CronJob execution, node diagnostics, ConfigMaps/Secrets, pod metrics, network inspection) are disabled in the Dynatrace provider. No `kubectl` is required. The original standalone prototype UI has been retired; tailg's shared viewer is the only interactive log implementation.

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

## F-key live log recording

In the actual tailg workload picker, highlight a service or select multiple with
Space, then press **F**. DynaR creates a timestamp-suffixed `dynar_*.ndjson`
file in the working directory and opens the shared tailg log viewer. The file
records only events timestamped **at or after the F keypress**; the initial
historical buffer is not written. Captures contain original raw messages,
workload, pod, container, timestamp, level, trace ID, and request ID, intended
for downstream AI investigation. Tailg's in-view recording path can be copied.

## Windows Terminal split panes (tailg-compatible)

```powershell
dynar -n apollo --split-panes
```

Pick several workloads with Space and press Enter. DynaR opens one Windows
Terminal pane per selected service with the same alternating vertical/horizontal
split arrangement as tailg. Each pane preserves the configured Dynatrace source,
cluster, lookback and search settings, with its own tailg log viewer.

Like tailg's separate-process panes, **each DynaR pane queries Dynatrace
independently**. For high-volume namespaces, opening a single multi-service
session is more API-efficient. The split feature requires Windows Terminal
(`wt.exe`) and Windows; WSL/Linux/macOS continue using the shared in-process
log viewer.

Split panes share a single tailg filter-state file, so typing a live filter and
pressing F1 in one pane synchronizes the text query and matches-only mode to
the others. DynaR uses the same filter synchronization implementation as tailg.
The picker F action always opens a single recording session rather than
launching split panes, preserving the live-only recording cutoff.

## Tailg-style interactive update dialog

The startup update check now opens the **actual tailg modal** with DynaR branding.
Tab/arrow keys select **Later** or **Update & restart**; Enter confirms, U updates,
and Esc continues without updating. The network update check is performed only
for stable interactive builds when `--no-update-check` is not specified.
DynaR continues to use **its own** GitHub release, SHA-256 verification, and
binary replacement logic; it never downloads or installs tailg as an update.

## F9 charts from Dynatrace HTTP log attributes

DynaR now forwards Grail HTTP method, route, response code and duration fields
into tailg's F9 rolling analytics when those fields are present on *individual
log records*. DQL explicitly requests standard attributes such as
`http.request.method`, `http.response.status_code`, `url.path`,
`duration` and `duration_ms`. Durations are included only when the returned
value includes a unit or the source field explicitly identifies milliseconds.

This is an **observed HTTP-completion log chart**, not a Dynatrace APM metric
or proof of all incoming traffic. If your application does not log HTTP
completion records with method/path/status or duration, F9 will show no HTTP
samples rather than manufacture request rates or latencies.

## Dynatrace-backed workload health and resource monitoring

```bash
dynar -n apollo                 # open tailg workload picker; press M to monitor
dynar -n apollo --monitor       # start monitoring immediately
dynar -n apollo --cluster tkgs-prod --monitor
```

**M** toggles a 30-second Dynatrace scan. The shared tailg picker shows log
health for each workload: gray for quiet, orange for warning events, red for
errors, flashing red for >=5 recent errors and amber/unknown for incomplete
logs. Monitoring never calls kubectl. **F9** opens the actual tailg resource
charts (CPU, memory and log alerts). **R** refreshes the monitoring snapshot.

CPU is deliberately shown as **% of the configured workload CPU limit** (not
arbitrarily converted millicores); memory is shown as observed working set
in MiB, optionally compared with the configured memory limit. When CPU limits
are missing, CPU% is **—** rather than 0. No resource reading is invented
when Dynatrace metrics are absent or a query fails.

Permissions: API tokens require `storage:metrics:read` and
`storage:buckets:read` for metrics; logs still require `storage:logs:read`.
For OAuth, DynaR uses an independent metrics access token so a missing metrics
permission does **not** prevent log-only operation. CPU/memory queries use
Dynatrace's Grail `dt.kubernetes.container.*` metrics, grouped by namespace
and workload with optional `--cluster` filtering. Monitoring data is scoped
to the log workloads discovered by DynaR. Missing log coverage or a saturated
local buffer cannot yield a healthy status.

Charts are telemetry observations, not direct pod readiness probes and not
proof of Kubernetes Deployment readiness. Monitor results may lag ingestion.

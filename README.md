# Homeland

**A tiny, self-hosted automation engine for Zigbee2MQTT - write your home automations in plain JavaScript.**

Homeland connects to your MQTT broker, tracks every device Zigbee2MQTT knows about, and runs small JavaScript handlers
you write in response to device events or on a schedule. No cloud, no YAML rule DSL, no heavyweight platform - just a
single binary (or container) that sits next to your broker and does one thing well.

```js
// scripts/livingroom-lamp-switch.js
function handle(state, ctx) {
  switch (state.action) {
    case 'on':
      ctx.z2m.publishProp('livingroom-lamp', 'state', 'ON');
      break;
    case 'off':
      ctx.z2m.publishProp('livingroom-lamp', 'state', 'OFF');
      break;
  }
}

module.exports = { handle };
```

That's a complete automation: when the switch reports `on`/`off`, the lamp follows.

---

## Why Homeland

- **Automations are just JavaScript.** A script is a small module that exports `handle(state, ctx)` and/or cron jobs. No new rule language to learn.
- **Runs anywhere small.** A single static binary on a scratch container. Happy on a Raspberry Pi or a mini PC next to your broker.
- **Scripts live on disk, not in a binary.** Drop a `<device>.js` file in a mounted directory. Optionally hot-reload on change.
- **Schedules built in.** Export `jobs` with standard cron syntax - turn things off at 22:00, nudge a device every morning.
- **Durable event log.** Every device event is persisted to an embedded SQLite DB, queryable through the API, with automatic retention.
- **Small HTTP API + UI.** Health, build info, the live device registry, per-device events, and the resolved handler for each device.
- **Forgiving by default.** Each script gets its own [goja](https://github.com/dop251/goja) ECMAScript runtime with no file,
  network or process access beyond `ctx`, and a timeout watchdog; a script that fails to compile never takes the service
  down. Scripts are still trusted code: goja is not a security sandbox.

## How it works

```
Sensor / Device --> Zigbee2MQTT --> MQTT broker --> Homeland --> Your JS handlers
                                                      |
                                                      |-> HTTP API + Web UI
                                                      |-> SQLite-persisted historical events
```

Homeland subscribes to Zigbee2MQTT device updates. On each update it queues the event for storage (written in the
background, in batches), then queues a run of the device's handler script. Each device's updates are handled in
arrival order; different devices run in parallel, so a slow script never holds up the others. Handlers publish
commands back to devices through `ctx.z2m`, read the latest state of any device with `ctx.z2m.getState`, or talk to
a second (non-z2m) broker via `ctx.mqtt`.

> **Zigbee2MQTT configuration.**
> For Homeland to work properly and always hold the latest state of every device (including ones that don't report
> for a while) your Zigbee2MQTT instance must retain device state. Set this in its `configuration.yaml`:
>
> ```yaml
> device_options:
>   retain: true
> ```

## Quick start (Docker Compose)

The fastest way to run the whole stack - Mosquitto, Zigbee2MQTT, and Homeland - is the example compose file
in [`docs/compose`](docs/compose/docker-compose.yaml).

```bash
# 1. Grab the example compose stack
#    (Mosquitto + Zigbee2MQTT + Homeland, with minimal configs for each)
cp -r docs/compose my-stack && cd my-stack

# 2. Point Zigbee2MQTT at your coordinator: the `devices:` line in docker-compose.yaml
#    and `serial.port` in zigbee2mqtt/data/configuration.yaml (e.g. /dev/ttyUSB0).
#    Set your timezone (TZ) in docker-compose.yaml.

# 3. Mosquitto and Homeland run as uid 1000. If `id -u` isn't 1000, hand them their dirs:
sudo chown -R 1000:1000 mosquitto homeland/data

docker compose up -d
```

Homeland's UI is then available at <http://localhost:8082/ui> (health at `/api/v1/health`), and Zigbee2MQTT's at
<http://localhost:8081>. The example broker allows anonymous connections, so keep it on a trusted network.

The minimal Homeland service on its own:

```yaml
services:
  homeland:
    image: ghcr.io/homeland-live/homeland:latest
    restart: always
    ports:
      - 8082:8080
    volumes:
      - ./homeland/scripts:/app/scripts   # your automation scripts
      - ./homeland/data:/app/data         # event database
    environment:
      TZ: Europe/Berlin
      HL_DB_PATH: /app/data/homeland.db
      HL_Z2M_MQTT_BROKER: tcp://mosquitto:1883
      HL_Z2M_MQTT_BASE_TOPIC: zigbee2mqtt
      HL_SCRIPTS_DIR: /app/scripts
```

> The image is published as a multi-arch manifest for `linux/amd64` and `linux/arm64` on GHCR as `ghcr.io/homeland-live/homeland`.

## Writing a handler

Each script is a JavaScript file in your scripts directory (`HL_SCRIPTS_DIR`), **named after the device** - either its
Zigbee2MQTT friendly name (`livingroom-lamp.js`) or its IEEE address (`0x00124b00247f9ca8.js`). A device may have only
one script: if both exist, neither runs, and the UI shows the device's script as broken until you remove one.

A script is a small CommonJS-style module: it sets `module.exports` to `{ handle, jobs }`, with a `handle`
function, at least one job, or both. It runs in strict mode with no host globals; everything it uses from Homeland
comes through `ctx`, and module-level variables persist between calls. A script with a syntax error or invalid
exports (for example an unknown key or a bad cron expression) is shown as broken in the UI and runs nothing.

### React to device events

Export `handle(state, ctx)`. It runs on every state update from that device, and `state` is the device's full
latest state (not just the changed properties).

```js
// scripts/hallway-sensor.js
function handle(state, ctx) {
  if (state.occupancy) {
    ctx.z2m.publishProp('hallway-light', 'state', 'ON');
  } else {
    ctx.z2m.publishProp('hallway-light', 'state', 'OFF');
  }
}

module.exports = { handle };
```

### React across devices

A handler can read the latest state of any other device:

```js
function handle(state, ctx) {
  const night = ctx.z2m.getState('bedroom-mode');
  if (night && night.state === 'ON') return; // do nothing at night
  ctx.z2m.publish('hallway-light', { state: 'ON', brightness: 200 });
}

module.exports = { handle };
```

### Run on a schedule

Export `jobs`, up to 20 per script: each one is `{ cron, run }`, where `run(ctx)` is called at every tick. `cron`
is standard 5-field cron or a descriptor such as `@daily` or `@every 10m`, evaluated in the process timezone
(`TZ`), overridable per expression with a `CRON_TZ=<zone>` prefix.

```js
// scripts/livingroom-lamp.js
function turnOffLamp(ctx) {
  ctx.z2m.publishProp('livingroom-lamp', 'state', 'OFF');
}

module.exports = {
  jobs: [{ cron: '0 22 * * *', run: turnOffLamp }],
};
```

### The handler API

| Call | Does |
| --- | --- |
| `ctx.z2m.publish(name, payload)` | Send a command object to a device's `set` topic |
| `ctx.z2m.publishProp(name, prop, value)` | Set a single property (e.g. `state`, `brightness`) |
| `ctx.z2m.getState(nameOrAddress)` | Read a device's latest reported state (or `null`) |
| `ctx.mqtt.publish(topic, payload)` | Publish to an arbitrary topic on the external broker |
| `ctx.log(...args)` | Log to the host (info level, tagged with the script name) |
| `ctx.sleep(ms)` | Pause the handler briefly (counts toward `HL_SCRIPTS_TIMEOUT`; the device's next events wait) |

Full, documented type declarations for authoring (with editor autocomplete) live in [`docs/script.d.ts`](docs/script.d.ts).
Its header shows how to type-check your scripts with a `jsconfig.json` next to them. More complete automations are in
the [example gallery](docs/README.md#example-scripts).

## Configuration

All configuration is via environment variables. A full annotated reference is in [`.env.example`](.env.example); the essentials:

| Variable | Default | Purpose |
| --- | --- | --- |
| `TZ` | - | Process timezone; also the default for cron schedules |
| `LOG_LEVEL` | - | Log verbosity (e.g. `debug`) |
| `HL_HTTP_PORT` | `8080` | Port of the API and UI |
| `HL_Z2M_MQTT_BROKER` | - | Zigbee2MQTT broker URL (e.g. `tcp://localhost:1883`) **(required)** |
| `HL_Z2M_MQTT_BASE_TOPIC` | `zigbee2mqtt` | Zigbee2MQTT base topic (its `mqtt.base_topic`) |
| `HL_Z2M_MQTT_CLIENT_ID` | `homeland-dev` | MQTT client id |
| `HL_Z2M_MQTT_USERNAME` / `_PASSWORD` | - | Broker credentials |
| `HL_Z2M_MQTT_READONLY` | `false` | Consume-only: publishes are logged, not sent |
| `HL_EXT_MQTT_BROKER` | - | Optional second broker for `ctx.mqtt` (non-z2m devices); unset disables `ctx.mqtt` |
| `HL_EXT_MQTT_BASE_TOPIC` | - | Optional prefix for every `ctx.mqtt.publish` topic |
| `HL_EXT_MQTT_CLIENT_ID` | `homeland-dev` | MQTT client id; must differ from `HL_Z2M_MQTT_CLIENT_ID` when both use the same broker (startup fails otherwise) |
| `HL_EXT_MQTT_USERNAME` / `_PASSWORD` / `_READONLY` | - / - / `false` | As for the Zigbee2MQTT broker |
| `HL_SCRIPTS_DIR` | `scripts` | Directory of handler scripts |
| `HL_SCRIPTS_WATCHER` | `false` | Hot-reload scripts on change |
| `HL_SCRIPTS_TIMEOUT` | `5s` | Bounds each script run: top-level code, every `handle` call and every scheduled job (`0` disables) |
| `HL_DB_PATH` | `data/homeland.db` | Embedded SQLite event store (WAL mode: keep it on a local disk, not NFS/SMB) |
| `HL_DB_RETENTION_MAX_AGE` | `720h` | Prune events older than this (`0` disables) |
| `HL_DB_RETENTION_MAX_PER_DEVICE` | `0` | Keep only newest N events per device (`0` = unlimited) |
| `HL_DB_RETENTION_INTERVAL` | `1h` | How often retention runs (also once at startup) |

## HTTP API

Served under `/api/v1`:

| Endpoint | Returns |
| --- | --- |
| `GET /health` | Composite health tree (503 when unhealthy) |
| `GET /status/buildinfo` | Build and runtime info |
| `GET /z2m/bridge/devices` | Device registry: spec, state schema, z2m device options, current state, resolved handler |
| `GET /z2m/bridge/info` | Zigbee2MQTT bridge info |
| `GET /z2m/bridge/health` | Zigbee2MQTT bridge health |
| `GET /z2m/devices/:id/events` | A device's stored events, latest first (`?pagenum=1&pagesize=10`, max 25), with its spec, state schema, z2m device options, current state and resolved handler |
| `GET /z2m/devices/:id/script` | A device's handler script |
| `GET /scripts` | Every loaded script with its handler; `orphan` flags a script whose name matches no device |

`:id` is a friendly name (URL-encoded) or an IEEE address. The static UI is served at `/ui`.

> **Security.** The HTTP API and UI have no authentication. They expose your device list, state, event history and
> the source of every script, so run Homeland on a trusted LAN only, and don't publish its port to the internet. Put
> it behind an authenticating reverse proxy if you need remote access.

## Building from source

Requires Go 1.27+ and Node (for the UI).

```bash
make build   # build ./bin/server
make dev     # run server + UI watcher together (goreman)
make test    # gotestsum: go test -race ./...
make lint    # golangci-lint
make ci      # lint + vet + test + docs-check
make docs    # regenerate the example gallery in docs/README.md
```

`make dev` is a long-running process - start it in your own terminal.

## Project layout

```
cmd/server/        entrypoint
cmd/gen-docs/      build-time tool: generates docs/README.md code blocks
internal/
  config/          environment-based configuration
  mqtt/            generic MQTT client (no z2m knowledge)
  z2m/             Zigbee2MQTT client: registry, state, publish, fan-out
  script/          goja JS runtime: load scripts, read their exports
  svc/scripts/     compile & cache handlers, optional fs watcher
  svc/event/       queue events for storage, resolve & run handlers
  svc/scheduler/   cron scheduling
  db/              SQLite event store + retention
  healthcheck/     composable health tree
  web/             Fiber HTTP server, API, static UI
docs/
  script.d.ts      handler type declarations
  README.md        docs index and example-script gallery
  examples/        example scripts the gallery is generated from
  compose/         example docker-compose stack
ui/                static web UI
```

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup and
conventions, and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for how the pieces fit together. Report security
issues privately as described in [SECURITY.md](SECURITY.md). Notable changes are listed in each GitHub release.

## License

Released under the [MIT License](LICENSE).

// Type declarations for Homeland device scripts.
//
// Each script under HL_SCRIPTS_DIR (default: scripts/) is a JavaScript file
// named "<device-id>.js", where the device id is the device's Zigbee2MQTT
// friendly name (e.g. "living-room-lamp.js") or its IEEE address
// (e.g. "0x00124b00247f9ca8.js"). Only files directly in the directory are
// loaded; hidden files (".#lamp.js" and other editor files) are ignored. A
// friendly name containing "/" cannot be a file name, so script such a
// device by its IEEE address.
//
// A script is a CommonJS-style module (see ScriptModule): it sets
// `module.exports` to `{ handle, jobs }`, with a `handle` function, at least
// one job, or both:
//
//     function handle(state, ctx) { ... }
//
//     function turnOffLamp(ctx) { ... }
//
//     module.exports = {
//       handle,
//       jobs: [{ cron: '0 22 * * *', run: turnOffLamp }],
//     };
//
// Setting properties on `exports` (`exports.handle = handle`) works too, but
// reassigning it (`exports = { ... }`) exports nothing. A device may have only
// one script: when it has one under both ids, neither script's `handle` or
// jobs run, and the API and UI report the device's script as broken.
//
// Runtime model:
//   - The host provides no globals: everything a script uses comes through
//     `ctx` (there is no `console`, `require` or `schedule`). Standard JS
//     built-ins such as `JSON`, `Math` and `Date` are available.
//   - A script runs in strict mode, so assigning to an undeclared variable
//     throws instead of creating a global.
//   - Each script gets its own runtime, created when the script loads (at
//     startup, or on change with HL_SCRIPTS_WATCHER=true). Its top-level code
//     runs once, and `module.exports` is read once, right after. Module-level
//     variables persist across `handle` calls and jobs; changing
//     `module.exports` later has no effect.
//   - Calls into one script (`handle` and its jobs) never overlap. A device's
//     events reach `handle` in arrival order; different devices' scripts run
//     in parallel. A job whose schedule fires while its previous run is still
//     going skips that run (and it is logged) rather than queueing it.
//   - HL_SCRIPTS_TIMEOUT (default 5s) bounds the top-level code, each `handle`
//     call and each job run, including any `ctx.sleep`. A run that times out
//     fails; at the top level that makes the script broken.
//   - A script that fails to load is reported as broken in the API and UI and
//     runs nothing; the rest keep working. Failing to load means a syntax
//     error, a top-level throw or timeout, or invalid exports: not an object,
//     an unknown key (a typo such as `job`), a `handle` that isn't a function,
//     an invalid job (see Job), or neither `handle` nor a job.
//   - Errors thrown during a run, including those thrown by ctx methods, are
//     logged. They never stop later runs.
//
// These declarations mirror the runtime contract implemented by the Go
// `internal/script` package (goja ECMAScript engine). They exist only for
// editor completion and type-checking; scripts don't import them.
//
// Usage: keep this file next to your scripts, add a jsconfig.json there:
//
//     { "compilerOptions": { "checkJs": true, "noEmit": true, "lib": ["es2020"],
//                            "module": "commonjs" },
//       "include": ["*.js", "script.d.ts"] }
//
// and check the exports with `@satisfies` (TypeScript 5.0+), which also types
// functions written inline. Annotate named functions with JSDoc:
//
//     /**
//      * @param {DeviceState} state
//      * @param {HandlerContext} ctx
//      */
//     function handle(state, ctx) { ... }
//
//     /** @param {HandlerContext} ctx */
//     function turnOffLamp(ctx) { ... }
//
//     module.exports = /** @satisfies {ScriptModule} */ ({
//       handle,
//       jobs: [{ cron: '0 22 * * *', run: turnOffLamp }],
//     });
//
// A JSDoc tag placed before `module.exports = ...` itself is not checked.

/**
 * Zigbee2MQTT device command and state API.
 *
 * The publish methods target a device by its Zigbee2MQTT friendly name and
 * publish to its `<friendly-name>/set[...]` topic under the Zigbee2MQTT base
 * topic (HL_Z2M_MQTT_BASE_TOPIC). With HL_Z2M_MQTT_READONLY=true they log
 * instead of sending. They throw when the broker connection is down.
 */
interface Z2M {
  /**
   * Send a command object to a device's `set` topic, e.g.
   * `ctx.z2m.publish("living-room-lamp", { state: "ON", brightness: 200 })`.
   *
   * @param friendlyName Device friendly name.
   * @param payload      Command object, sent as JSON.
   */
  publish(friendlyName: string, payload: Record<string, unknown>): void;

  /**
   * Set a single property on a device via the `set/<prop>` topic, e.g.
   * `ctx.z2m.publishProp("living-room-lamp", "state", "OFF")`.
   *
   * Strings are sent verbatim (unquoted); other values are JSON-encoded.
   *
   * @param friendlyName Device friendly name.
   * @param prop         Property name (e.g. "state", "brightness").
   * @param value        Property value.
   */
  publishProp(friendlyName: string, prop: string, value: unknown): void;

  /**
   * Read the latest reported state of any device by its Zigbee2MQTT friendly
   * name or IEEE address, e.g. `ctx.z2m.getState("hallway-sensor")`. This lets
   * a handler react to devices other than the one that triggered the event.
   *
   * Returns `null` when the device is unknown or has not reported state yet,
   * so the result can be tested directly.
   *
   * @param friendlyNameOrAddress Device friendly name or IEEE address.
   */
  getState(friendlyNameOrAddress: string): DeviceState | null;
}

/**
 * MQTT publish API for non-Zigbee2MQTT devices, on the external broker
 * (HL_EXT_MQTT_BROKER), which may differ from the Zigbee2MQTT one.
 *
 * Only available when an external broker is configured; otherwise
 * `publish` throws. With HL_EXT_MQTT_READONLY=true it logs instead of sending.
 * It throws when the broker connection is down.
 */
interface MQTT {
  /**
   * Publish a payload to a topic, e.g.
   * `ctx.mqtt.publish("awtrix/notify", { text: "Door open" })`.
   *
   * The payload is always JSON-encoded, so a string is sent with its quotes
   * (`"hello"`), unlike `ctx.z2m.publishProp`.
   *
   * @param topic   Topic to publish to. When HL_EXT_MQTT_BASE_TOPIC is set,
   *                it is prefixed as `<base>/<topic>`.
   * @param payload Any JSON-serializable value.
   */
  publish(topic: string, payload: unknown): void;
}

/**
 * A device's full latest state (not just the changed attributes): the first
 * argument to `handle`, and what `ctx.z2m.getState` returns.
 *
 * A plain object keyed by attribute name, decoded from the state Zigbee2MQTT
 * reported, without its `device` and `update` attributes. The exact
 * attributes depend on the device; values are strings, numbers, booleans, or
 * nested objects/arrays. Common attributes are surfaced below for
 * convenience, but any attribute may be accessed via the index signature.
 */
interface DeviceState {
  /** Button/switch action reported by the device (e.g. "on", "off", "toggle", "brightness_step_up"). */
  action?: string;
  /** On/off state, typically "ON" or "OFF". */
  state?: string;
  /** Battery percentage, if reported. */
  battery?: number;
  /** Zigbee link quality (LQI), if reported. */
  linkquality?: number;

  /** Any other device-specific attribute. */
  [attribute: string]: unknown;
}

/**
 * Host context passed to `handle` as its second argument and to every job's
 * `run`.
 */
interface HandlerContext {
  /**
   * Log to the host, e.g. `ctx.log("action:", state.action)`. Arguments are
   * joined with spaces like `console.log` (strings verbatim, objects and
   * arrays as JSON) and written at info level, tagged with the script's file
   * name, so the host's `LOG_LEVEL` applies. There is no `console`.
   *
   * @param args Values to log.
   */
  log(...args: unknown[]): void;

  /**
   * Block the run for the given number of milliseconds, e.g.
   * `ctx.sleep(500)` to space out commands.
   *
   * The sleep counts toward HL_SCRIPTS_TIMEOUT (default 5s): when the run
   * times out, the sleep is cut short and the run fails. While a `handle`
   * call sleeps, the device's next events wait: up to 64 are queued and
   * later ones are dropped (and logged). Other devices are not held up. Keep
   * sleeps short. A non-positive argument returns immediately.
   *
   * @param ms Milliseconds to sleep.
   */
  sleep(ms: number): void;

  /** Zigbee2MQTT device command and state API. */
  z2m: Z2M;

  /** MQTT publish API for non-Zigbee2MQTT devices (external broker). */
  mqtt: MQTT;
}

/**
 * Device event handler. When a script exports it, it runs on the device's
 * state updates, except for:
 *   - the first state after Homeland (re)starts, which is the initial
 *     sync-up, not an event;
 *   - a state equal to the previous one apart from `last_seen`,
 *     `linkquality` and `action` (one that carries `action` always counts,
 *     so repeated button presses do);
 *   - a state missing one of the device's primary attributes (e.g. `state`
 *     for a light), which is incomplete. `action` counts only for a device
 *     whose sole primary attribute it is, such as a remote: then only
 *     presses reach `handle`;
 *   - a press replayed by the broker after a reconnect: the state it
 *     arrived with still counts if it changed, without `action`.
 *
 * @param state The device's full latest state.
 * @param ctx   Host APIs: log, sleep, z2m and mqtt.
 */
type Handle = (state: DeviceState, ctx: HandlerContext) => void;

/**
 * A cron-scheduled job. `run` is called at every tick of `cron`,
 * independently of device events, with the same host context `handle` gets.
 *
 * `cron` uses standard 5-field crontab syntax (minute hour day-of-month month
 * day-of-week; no seconds field) or a descriptor such as `@hourly`, `@daily`
 * or `@every 10m`. It is evaluated in the host's local timezone (the `TZ`
 * environment variable), unless prefixed with `TZ=<zone> ` or
 * `CRON_TZ=<zone> `, e.g. `"CRON_TZ=Europe/Berlin 0 7 * * *"`.
 *
 * A job is checked when the script loads: an invalid expression, a missing
 * `cron` or `run`, or any other key makes the whole script broken.
 */
interface Job {
  /** Crontab expression or descriptor, optionally TZ-prefixed. */
  cron: string;
  /** Callback invoked as `run(ctx)` at each tick. */
  run(ctx: HandlerContext): void;
}

/**
 * What a script assigns to `module.exports`. At least one of `handle` and
 * `jobs` is required, and no other keys are allowed.
 */
interface ScriptModule {
  /** Runs on the device's state updates (see Handle). */
  handle?: Handle;
  /**
   * Cron-scheduled jobs (see Job). At most 20; more makes the whole script
   * broken.
   */
  jobs?: Job[];
}

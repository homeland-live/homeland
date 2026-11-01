# Homeland docs

Reference material for running Homeland and writing automations.

- [`script.d.ts`](script.d.ts) — the authoring contract for device scripts:
  TypeScript declarations for the `state` shape, the `ctx` API, the module
  format, and the runtime model (timeouts, event filtering, per-script
  runtime). Its header shows how to type-check your scripts with a
  `jsconfig.json`, and [`tsconfig.script.json`](tsconfig.script.json) is the
  project's own check.
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — how the services and data flow fit
  together.
- [`compose/`](compose/) — an example Docker Compose stack (Mosquitto,
  Zigbee2MQTT, Homeland).
- [Example scripts](#example-scripts) — a gallery of complete automations,
  below.

See the ["Writing a handler"](../README.md#writing-a-handler) section of the
main README for a guided introduction.

## Example scripts

A gallery of small, self-contained Homeland automations, one per use case. Each
block below is a complete script, headed by its file name, and opens with a
comment describing what it does and how it is triggered. Copy it into a file in
your scripts directory (`HL_SCRIPTS_DIR`), name the file after your device, and
rename the device names inside to match your own.

A script file is named after a device — its Zigbee2MQTT friendly name
(`hallway-sensor.js`) or its IEEE address (`0x00124b00247f9ca8.js`) — and runs
on that device's events and/or on a schedule. A device may have only one
script. For the full authoring contract (the `state` shape, the `ctx` API,
timeouts, how events are filtered), see [`script.d.ts`](script.d.ts).

To get editor autocomplete and type-checking, keep `script.d.ts` next to your
scripts and add the `@satisfies {ScriptModule}` and JSDoc annotations its
header describes; Homeland itself ignores them. The examples below carry those
annotations because they are the real, type-checked files under
[`examples/`](examples); this gallery is generated from them, so edit the
files, not this document (see [Regenerating](#regenerating)).

<!-- gen-docs:begin examples/*.js -->
### bathroom-sensor.js

```js
/*
 * React and run on a schedule.
 *
 * Trigger: device events and a schedule.
 *
 * The file is named after the motion sensor, which turns the light on and
 * off, and a nightly job forces the light off as a backstop in case the
 * "clear" event is missed. A device may have only one script, so a device
 * that should both react and run on a schedule exports `handle` and `jobs`
 * together from its single file.
 */

/**
 * @param {HandlerContext} ctx
 * @param {DeviceState} state
 */
function handle(ctx, state) {
  if (state.occupancy) {
    ctx.z2m.publish('bathroom-light', { state: 'ON', brightness: 200 });
  } else {
    ctx.z2m.publishProp('bathroom-light', 'state', 'OFF');
  }
}

/** @param {HandlerContext} ctx */
function forceOff(ctx) {
  ctx.z2m.publishProp('bathroom-light', 'state', 'OFF');
}

module.exports = /** @satisfies {ScriptModule} */ ({
  handle,
  jobs: [{ cron: '0 1 * * *', run: forceOff }],
});
```

---

### bedroom-switch.js

```js
/*
 * Map remote buttons to commands.
 *
 * Trigger: device events.
 *
 * Turns a battery remote's button presses into light commands. A remote's
 * only primary property is `action`, so `handle` runs once per press.
 * `publishProp` sets a single property; `publish` sends a whole command
 * object (here a relative `brightness_step`). Unknown actions are logged with
 * `ctx.log` and ignored.
 */

/**
 * @param {HandlerContext} ctx
 * @param {DeviceState} state
 */
function handle(ctx, state) {
  switch (state.action) {
    case 'on':
      ctx.z2m.publishProp('bedroom-lamp', 'state', 'ON');
      break;
    case 'off':
      ctx.z2m.publishProp('bedroom-lamp', 'state', 'OFF');
      break;
    case 'brightness_step_up':
      ctx.z2m.publish('bedroom-lamp', { brightness_step: 40 });
      break;
    case 'brightness_step_down':
      ctx.z2m.publish('bedroom-lamp', { brightness_step: -40 });
      break;
    default:
      ctx.log(`ignore "${state.action}" action`);
  }
}

module.exports = /** @satisfies {ScriptModule} */ ({ handle });
```

---

### front-door.js

```js
/*
 * Notify a non-Zigbee2MQTT device.
 *
 * Trigger: device events.
 *
 * When the contact sensor opens (`contact: false`), pushes a notification to
 * an MQTT display over the external broker with `ctx.mqtt.publish`. This
 * needs `HL_EXT_MQTT_BROKER` configured; without it, `ctx.mqtt.publish`
 * throws (the host catches and logs it, and the service keeps running).
 * Unlike `publishProp`, `ctx.mqtt.publish` always JSON-encodes its payload.
 */

/**
 * @param {HandlerContext} ctx
 * @param {DeviceState} state
 */
function handle(ctx, state) {
  if (state.contact === false) {
    // contact:false means the door is open.
    ctx.mqtt.publish('awtrix/notify', { text: 'Front door opened', color: '#ff0000' });
  }
}

module.exports = /** @satisfies {ScriptModule} */ ({ handle });
```

---

### hallway-sensor.js

```js
/*
 * Mirror a sensor onto a light.
 *
 * Trigger: device events.
 *
 * Follows a motion/occupancy sensor with a light. The file is named after the
 * sensor (the device whose events drive the automation) and publishes to a
 * separate light device by name. This is the simplest shape: one device
 * reacts, another is commanded.
 *
 * `handle(ctx, state)` receives the device's full latest state, not just the
 * changed properties, so it can read whatever the device reports.
 */

/**
 * @param {HandlerContext} ctx
 * @param {DeviceState} state
 */
function handle(ctx, state) {
  if (state.occupancy) {
    ctx.z2m.publishProp('hallway-light', 'state', 'ON');
  } else {
    ctx.z2m.publishProp('hallway-light', 'state', 'OFF');
  }
}

module.exports = /** @satisfies {ScriptModule} */ ({ handle });
```

---

### kitchen-sensor.js

```js
/*
 * Decide from another device's state.
 *
 * Trigger: device events.
 *
 * The motion sensor drives the light, but a "night mode" helper device
 * suppresses it. `ctx.z2m.getState` reads any device's latest reported state
 * by friendly name or IEEE address, and returns `null` when the device is
 * unknown or hasn't reported yet, so a null check is enough before using it.
 */

/**
 * @param {HandlerContext} ctx
 * @param {DeviceState} state
 */
function handle(ctx, state) {
  if (!state.occupancy) {
    ctx.z2m.publishProp('kitchen-light', 'state', 'OFF');
    return;
  }

  const night = ctx.z2m.getState('night-mode');
  if (night && night.state === 'ON') {
    ctx.log('night mode on, leaving kitchen light off');
    return;
  }

  ctx.z2m.publish('kitchen-light', { state: 'ON', brightness: 254 });
}

module.exports = /** @satisfies {ScriptModule} */ ({ handle });
```

---

### office-sensor.js

```js
/*
 * React only when a property changed.
 *
 * Trigger: device events.
 *
 * A presence sensor also reports battery, illuminance and other properties,
 * and each report runs `handle` with the full state. Its `prevState` argument
 * is the device's state just before this one, so comparing the two acts only
 * when presence actually flips, not on every report. `prevState` is `null`
 * when there is no previous state, so check it first.
 */

/**
 * @param {HandlerContext} ctx
 * @param {DeviceState} state
 * @param {DeviceState | null} prevState
 */
function handle(ctx, state, prevState) {
  if (!prevState || prevState.presence === state.presence) {
    return;
  }

  if (state.presence) {
    ctx.log('office occupied');
    ctx.z2m.publishProp('office-light', 'state', 'ON');
  } else {
    ctx.log('office empty');
    ctx.z2m.publishProp('office-light', 'state', 'OFF');
  }
}

module.exports = /** @satisfies {ScriptModule} */ ({ handle });
```

---

### porch-light.js

```js
/*
 * Run on a schedule.
 *
 * Trigger: schedule only, no device events.
 *
 * Exports `jobs` (up to 20 per script) and no `handle`. Each job is
 * `{ cron, run }`; `run(ctx)` is called at every tick. `cron` is standard
 * 5-field crontab syntax or a descriptor such as `@daily` or `@every 10m`,
 * evaluated in the process timezone (`TZ`) unless prefixed with `TZ=<zone>`
 * or `CRON_TZ=<zone>`.
 */

/** @param {HandlerContext} ctx */
function turnOn(ctx) {
  ctx.z2m.publishProp('porch-light', 'state', 'ON');
}

/** @param {HandlerContext} ctx */
function turnOff(ctx) {
  ctx.z2m.publishProp('porch-light', 'state', 'OFF');
}

module.exports = /** @satisfies {ScriptModule} */ ({
  jobs: [
    { cron: '0 18 * * *', run: turnOn },
    { cron: '30 0 * * *', run: turnOff },
  ],
});
```
<!-- gen-docs:end -->

## Regenerating

The gallery above is generated from the real `.js` files in
[`examples/`](examples) by `cmd/gen-docs`, so the snippets and the
type-checked files never drift. Generated content sits between
`<!-- gen-docs:begin <path> -->` and `<!-- gen-docs:end -->` markers, and the
generator replaces whatever is between them. A file path becomes a fenced
block of that file; a glob such as `examples/*.js` becomes a `###` heading
and fenced block per matching file, in alphabetical order and separated by
horizontal rules. Paths are relative to `docs/`. To add an example, drop a
new `.js` file into `examples/` and regenerate.

After editing an example (or this document's markers), regenerate and verify:

```bash
make docs        # rewrite the generated blocks in docs/README.md
make docs-check  # fail if docs/README.md is out of date (used in CI)
```

To format the examples (then regenerate) or type-check them against
`script.d.ts`, run `make docs-examples-fmt` or `make docs-examples-ts`;
`make docs-examples-fmt-check` checks the formatting without writing. They
cover the [`compose/`](compose/) scripts too and use the UI's tools
(`ui/node_modules`, installed on first use). CI runs both checks, plus
`make docs-check`. The targets live in [`targets.mk`](targets.mk).

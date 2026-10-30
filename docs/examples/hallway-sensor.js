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
 * `handle(state, ctx)` receives the device's full latest state, not just the
 * changed properties, so it can read whatever the device reports.
 */

/**
 * @param {DeviceState} state
 * @param {HandlerContext} ctx
 */
function handle(state, ctx) {
  if (state.occupancy) {
    ctx.z2m.publishProp('hallway-light', 'state', 'ON');
  } else {
    ctx.z2m.publishProp('hallway-light', 'state', 'OFF');
  }
}

module.exports = /** @satisfies {ScriptModule} */ ({ handle });

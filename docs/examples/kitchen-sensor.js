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
 * @param {DeviceState} state
 * @param {HandlerContext} ctx
 */
function handle(state, ctx) {
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

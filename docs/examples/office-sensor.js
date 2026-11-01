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

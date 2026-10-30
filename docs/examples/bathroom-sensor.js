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
 * @param {DeviceState} state
 * @param {HandlerContext} ctx
 */
function handle(state, ctx) {
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

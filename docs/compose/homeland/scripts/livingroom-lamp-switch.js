/*
 * Toggle the living room lamp from a wall switch.
 *
 * Trigger: device events.
 *
 * The switch's `on`/`off` actions set the lamp's state; any other action is
 * logged and ignored.
 */

/**
 * @param {HandlerContext} ctx
 * @param {DeviceState} state
 */
function handle(ctx, state) {
  switch (state.action) {
    case 'on':
      ctx.z2m.publishProp('livingroom-lamp', 'state', 'ON');
      break;
    case 'off':
      ctx.z2m.publishProp('livingroom-lamp', 'state', 'OFF');
      break;
    default:
      ctx.log(`ignore "${state.action}" switch action`);
  }
}

module.exports = /** @satisfies {ScriptModule} */ ({ handle });

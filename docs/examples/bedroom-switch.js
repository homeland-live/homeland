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
 * @param {DeviceState} state
 * @param {HandlerContext} ctx
 */
function handle(state, ctx) {
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

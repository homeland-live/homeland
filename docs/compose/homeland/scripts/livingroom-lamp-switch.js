/**
 * @param {DeviceState} state
 * @param {HandlerContext} ctx
 */
function handle(state, ctx) {
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

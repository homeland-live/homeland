// Turn the lamp off every day at 22:00 (in the container's TZ).

/** @param {HandlerContext} ctx */
function turnOffLamp(ctx) {
  ctx.z2m.publishProp('livingroom-lamp', 'state', 'OFF');
}

module.exports = /** @satisfies {ScriptModule} */ ({
  jobs: [{ cron: '0 22 * * *', run: turnOffLamp }],
});

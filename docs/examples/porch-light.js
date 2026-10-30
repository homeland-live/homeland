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

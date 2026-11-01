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

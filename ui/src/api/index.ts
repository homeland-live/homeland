export interface ErrItem {
  field: string;
  msg: string;
}

export interface Err {
  code: number;
  msg: string;
  items?: ErrItem[];
}

function headers(): Record<string, string> {
  return {
    Accept: 'application/json',
    'Content-Type': 'application/json',
  };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function handleResponse(resp: Response): Promise<any> {
  if (!resp.ok) {
    return resp.json().catch(() => ({ error: { code: resp.status, msg: resp.statusText } }));
  }
  return resp.json().catch(() => ({ error: { code: resp.status, msg: 'Invalid JSON response' } }));
}

function handleNetworkError(err: unknown): { error: Err } {
  return { error: { code: 0, msg: err instanceof Error ? err.message : 'Network error' } };
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function get(url: string): Promise<any> {
  return fetch(url, {
    headers: headers(),
  })
    .then(handleResponse)
    .catch(handleNetworkError);
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function post(url: string, data: Record<string, unknown>): Promise<any> {
  return fetch(url, {
    method: 'POST',
    headers: headers(),
    body: JSON.stringify(data),
  })
    .then(handleResponse)
    .catch(handleNetworkError);
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function patch(url: string, data: Record<string, unknown>): Promise<any> {
  return fetch(url, {
    method: 'PATCH',
    headers: headers(),
    body: JSON.stringify(data),
  })
    .then(handleResponse)
    .catch(handleNetworkError);
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function del(url: string, data?: Record<string, unknown>): Promise<any> {
  const options: Record<string, unknown> = {
    method: 'DELETE',
    headers: headers(),
  };
  if (data) {
    options.body = JSON.stringify(data);
  }
  return fetch(url, options).then((resp) => resp.json());
}

// Page is the pagination envelope returned by list endpoints. `count` is the
// total number of matching rows, so the UI can derive the number of pages.
export interface Page {
  num: number;
  size: number;
  count: number;
}

export interface PageQuery {
  pagenum: number;
  pagesize: number;
}

export interface Dep {
  name: string;
  version: string;
}

export function getDeps(name: string): Promise<Dep[]> {
  return get(`/ui/${name}-deps.json?t=${Date.now()}`).then((data) => data.deps || []);
}

// HealthCheck is one node of the server's health tree. A parent is healthy
// only when every child is; `error` is set on an unhealthy node (for a parent
// it names the failing children). Leaves have no `checks`.
export interface HealthCheck {
  name: string;
  isHealthy: boolean;
  error?: string;
  checks?: HealthCheck[];
}

// getHealth fetches the health tree. The server answers 503 with the same tree
// when unhealthy, so only a response that isn't a tree (network failure,
// invalid JSON) is reported as `error`.
export function getHealth(): Promise<{ health?: HealthCheck; error?: Err }> {
  return get('/api/v1/health').then((data) =>
    typeof data?.isHealthy === 'boolean'
      ? { health: data as HealthCheck }
      : { error: data?.error ?? { code: 0, msg: 'Invalid health response' } },
  );
}

export interface BuildInfo {
  compiler: string;
  os: string;
  arch: string;
  commit: string;
  ref: string;
  buildTimeUTC: string;
  buildTimeLocal: string;
  serverUptime: string;
  serverTime: string;
  serverTimezone: string;
}

export function getStatusBuildInfo(): Promise<BuildInfo> {
  return get('/api/v1/status/buildinfo');
}

// Z2MExpose describes a capability a device exposes (a readable/writable
// property). Composite exposes nest sub-features under `features`. Only the
// fields the UI uses are modelled; z2m publishes more per expose.
export interface Z2MExpose {
  type: string;
  name: string;
  property: string;
  label: string;
  description: string;
  unit: string;
  access: number;
  features?: Z2MExpose[];
}

export interface Z2MDeviceDefinition {
  source: string;
  model: string;
  vendor: string;
  description: string;
  exposes: Z2MExpose[];
}

export interface Z2MDeviceSpec {
  ieee_address: string;
  type: string;
  network_address: number;
  supported: boolean;
  disabled: boolean;
  friendly_name: string;
  description: string;
  power_source: string;
  date_code: string;
  model_id: string;
  interview_state: string;
  definition: Z2MDeviceDefinition | null;
}

// Z2MStateSchema partitions a device's published state properties by expose
// category. `primaryProps` are the regular-usage properties (switch, light,
// sensor readings, button `action`); `secondaryProps` are the config and
// diagnostic properties (operation_mode, power_on_behavior, battery,
// linkquality, last_seen, ...). Each bucket maps a property name to its expose
// type (binary, numeric, enum, composite, list, ...); the injected last_seen
// prop carries an empty type. Derived from the device definition, so it is
// null when the device has no definition.
export interface Z2MStateSchema {
  primaryProps: Record<string, string>;
  secondaryProps: Record<string, string>;
}

// Handler describes a device's compiled script as the API serializes it.
// `hasHandle` is true when the script exports a handle(ctx, state) function
// (it reacts to device events); `jobCount` is how many jobs it exports (it
// runs on a cron schedule). A script may do both. `error` is
// set when the script failed to compile, in which case neither the handle nor
// the jobs run and both `hasHandle` and `jobCount` are their zero values.
export interface Handler {
  hasHandle: boolean;
  jobCount: number;
  error?: Err;
}

// Z2MDevice pairs a device's static metadata (spec) with its latest reported
// state. `state` is a free-form property map (null until state is seen).
// `options` are the z2m options that apply to the device (its own merged over
// device_options), null until bridge/info arrives and when none are set.
export interface Z2MDevice {
  spec: Z2MDeviceSpec;
  schema: Z2MStateSchema;
  options: Z2MDeviceOptions | null;
  state: Record<string, unknown> | null;
  handler: Handler | null;
}

export function getZ2MBridgeDevices(): Promise<{ devices?: Z2MDevice[]; error?: Err }> {
  return get('/api/v1/z2m/bridge/devices');
}

export interface Z2MBridgeCoordinator {
  ieee_address: string;
  type: string;
}

export interface Z2MBridgeNetwork {
  channel: number;
  pan_id: number;
  // z2m publishes extended_pan_id inconsistently (byte array or hex string),
  // so it is kept as an unknown raw value.
  extended_pan_id: unknown;
}

export interface Z2MBridgeVersion {
  version: string;
}

export interface Z2MBridgeInfoOS {
  version: string;
  node_version: string;
  cpus: string;
  memory_mb: number;
}

export interface Z2MBridgeInfoMQTT {
  server: string;
  version: number;
}

// Z2MDeviceOptions is the part of a z2m device's options Homeland reads.
// filtered_attributes is null when unset.
export interface Z2MDeviceOptions {
  filtered_attributes: string[] | null;
}

// Z2MBridgeConfig is the part of z2m's settings Homeland reads. devices is
// keyed by IEEE address and null when bridge/info carries no config.
export interface Z2MBridgeConfig {
  device_options: Z2MDeviceOptions;
  devices: Record<string, Z2MDeviceOptions> | null;
}

export interface Z2MBridgeInfo {
  version: string;
  commit: string;
  coordinator: Z2MBridgeCoordinator;
  network: Z2MBridgeNetwork;
  log_level: string;
  permit_join: boolean;
  permit_join_end: number | null;
  restart_required: boolean;
  zigbee_herdsman: Z2MBridgeVersion;
  zigbee_herdsman_converters: Z2MBridgeVersion;
  os: Z2MBridgeInfoOS;
  mqtt: Z2MBridgeInfoMQTT;
  config: Z2MBridgeConfig;
}

export interface Z2MBridgeHealthOS {
  load_average: number[];
  memory_used_mb: number;
  memory_percent: number;
}

export interface Z2MBridgeHealthProcess {
  uptime_sec: number;
  memory_used_mb: number;
  memory_percent: number;
}

export interface Z2MBridgeHealthMQTT {
  connected: boolean;
  queued: number;
  published: number;
  received: number;
}

export interface Z2MBridgeHealthDevice {
  leave_count: number;
  network_address_changes: number;
  messages: number;
  messages_per_sec: number;
}

export interface Z2MBridgeHealth {
  response_time: number;
  os: Z2MBridgeHealthOS;
  process: Z2MBridgeHealthProcess;
  mqtt: Z2MBridgeHealthMQTT;
  devices: Record<string, Z2MBridgeHealthDevice>;
}

export function getZ2MBridgeInfo(): Promise<{ info?: Z2MBridgeInfo; error?: Err }> {
  return get('/api/v1/z2m/bridge/info');
}

export function getZ2MBridgeHealth(): Promise<{ health?: Z2MBridgeHealth; error?: Err }> {
  return get('/api/v1/z2m/bridge/health');
}

// Event is a single persisted device event: the raw payload reported by the
// device plus when it was stored. Keyed by IEEE address (stable identity), so
// events stay attributable across a friendly-name rename.
export interface Event {
  id: number;
  ieeeAddress: string;
  payload: Record<string, unknown>;
  createdAt: string;
}

// DeviceState is a device's latest reported state: a unique id, the time it
// was recorded, and the free-form property map (`props`). `id`/`createdAt`
// let the UI show when the current state was captured.
export interface DeviceState {
  id: string;
  createdAt: string;
  props: Record<string, unknown> | null;
}

// DeviceEventsResponse carries a page of a device's stored events plus the
// device's current metadata (spec), state schema, z2m options, state, and
// handler, so the detail page can render them from one request.
// spec/schema/state are null when the device is not known to the bridge (its
// stored events are still returned); options are null until bridge/info
// arrives and when none are set.
export interface DeviceEventsResponse {
  spec: Z2MDeviceSpec | null;
  schema: Z2MStateSchema | null;
  options: Z2MDeviceOptions | null;
  state: DeviceState | null;
  handler: Handler | null;
  events?: Event[];
  page?: Page;
  error?: Err;
}

// listDeviceEvents fetches a page of a device's stored events (latest first)
// together with the device's metadata and current state. The device is
// identified by its IEEE address (stable identity).
export function listDeviceEvents(id: string, query: PageQuery): Promise<DeviceEventsResponse> {
  const params = new URLSearchParams({
    pagenum: String(query.pagenum),
    pagesize: String(query.pagesize),
  });
  return get(`/api/v1/z2m/devices/${encodeURIComponent(id)}/events?${params}`);
}

// Script is a device's user-defined event handler source. `content` is the
// raw source text. `deviceId` is the device identity the script is keyed by (a
// friendly name or an IEEE address), i.e. the file name without its ".js"
// extension.
export interface Script {
  filename: string;
  content: string;
  deviceId: string;
}

// DeviceScriptResponse carries the device's source script, or null when the
// device has no script.
export interface DeviceScriptResponse {
  script: Script | null;
  error?: Err;
}

// getDeviceScript fetches the source script for a device, identified by its
// IEEE address. `script` is null when the device has no script.
export function getDeviceScript(id: string): Promise<DeviceScriptResponse> {
  return get(`/api/v1/z2m/devices/${encodeURIComponent(id)}/script`);
}

// CompiledScript is one loaded script in the scripts listing. `deviceId` is
// the file name without ".js" (a friendly name or an IEEE address). `orphan`
// is true when that id names no device known to the bridge, i.e. a typo or a
// stale name, so the script never runs. `script`/`handler` carry the source
// and compiled status (handler.error is set when it failed to compile).
export interface CompiledScript {
  deviceId: string;
  script: Script | null;
  handler: Handler | null;
  orphan: boolean;
}

// listScripts fetches every loaded script, including orphans whose device id
// matches no known device. Unlike getDeviceScript, it enumerates the scripts
// directory rather than resolving from a device.
export function listScripts(): Promise<{ scripts?: CompiledScript[]; error?: Err }> {
  return get('/api/v1/scripts');
}

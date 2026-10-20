package z2m

import "encoding/json"

// BridgeState represents the payload of the `zigbee2mqtt/bridge/state` topic.
type BridgeState struct {
	State string `json:"state"` // "online" | "offline"
}

// BridgeInfo represents the payload of the `zigbee2mqtt/bridge/info` topic. It
// is republished whenever one of its attributes changes. Only the fields we
// care about are modelled; z2m publishes more (config, config_schema, ...).
type BridgeInfo struct {
	Version               string            `json:"version"`
	Commit                string            `json:"commit"`
	Coordinator           BridgeCoordinator `json:"coordinator"`
	Network               BridgeNetwork     `json:"network"`
	LogLevel              string            `json:"log_level"`
	PermitJoin            bool              `json:"permit_join"`
	PermitJoinEnd         *int64            `json:"permit_join_end"`
	RestartRequired       bool              `json:"restart_required"`
	ZigbeeHerdsman        BridgeVersion     `json:"zigbee_herdsman"`
	ZigbeeHerdsmanConvert BridgeVersion     `json:"zigbee_herdsman_converters"`
	OS                    BridgeInfoOS      `json:"os"`
	MQTT                  BridgeInfoMQTT    `json:"mqtt"`
}

// BridgeVersion is a simple {"version": ...} sub-object used by bridge/info.
type BridgeVersion struct {
	Version string `json:"version"`
}

// BridgeCoordinator describes the coordinator radio reported in bridge/info.
type BridgeCoordinator struct {
	IEEEAddress string `json:"ieee_address"`
	Type        string `json:"type"`
}

// BridgeNetwork describes the Zigbee network parameters reported in bridge/info.
type BridgeNetwork struct {
	Channel int `json:"channel"`
	PanID   int `json:"pan_id"`
	// ExtendedPanID is kept raw because z2m publishes it inconsistently across
	// versions/configs: as a byte array (e.g. [0,11,22]) or as a hex string
	// (e.g. "0x00124b00..."). Decoding it into a fixed type would fail on the
	// other form; callers can interpret the raw JSON as needed.
	ExtendedPanID json.RawMessage `json:"extended_pan_id"`
}

// BridgeInfoOS describes the host OS reported in bridge/info.
type BridgeInfoOS struct {
	Version     string `json:"version"`
	NodeVersion string `json:"node_version"`
	CPUs        string `json:"cpus"`
	MemoryMB    int    `json:"memory_mb"`
}

// BridgeInfoMQTT describes the MQTT connection reported in bridge/info.
type BridgeInfoMQTT struct {
	Server  string `json:"server"`
	Version int    `json:"version"`
}

// BridgeHealth represents the payload of the `zigbee2mqtt/bridge/health` topic,
// published periodically (default every 10 minutes; see z2m health config).
type BridgeHealth struct {
	ResponseTime int64                         `json:"response_time"` // Unix ms
	OS           BridgeHealthOS                `json:"os"`
	Process      BridgeHealthProcess           `json:"process"`
	MQTT         BridgeHealthMQTT              `json:"mqtt"`
	Devices      map[string]BridgeHealthDevice `json:"devices"` // keyed by IEEE address
}

// BridgeHealthOS holds system-level health metrics from bridge/health.
type BridgeHealthOS struct {
	LoadAverage   []float64 `json:"load_average"` // [1m, 5m, 15m]
	MemoryUsedMB  float64   `json:"memory_used_mb"`
	MemoryPercent float64   `json:"memory_percent"`
}

// BridgeHealthProcess holds Zigbee2MQTT process health metrics from bridge/health.
type BridgeHealthProcess struct {
	UptimeSec     int64   `json:"uptime_sec"`
	MemoryUsedMB  float64 `json:"memory_used_mb"`
	MemoryPercent float64 `json:"memory_percent"`
}

// BridgeHealthMQTT holds MQTT health metrics from bridge/health.
type BridgeHealthMQTT struct {
	Connected bool `json:"connected"`
	Queued    int  `json:"queued"`
	Published int  `json:"published"`
	Received  int  `json:"received"`
}

// BridgeHealthDevice holds per-device health metrics from bridge/health.
type BridgeHealthDevice struct {
	LeaveCount            int     `json:"leave_count"`
	NetworkAddressChanges int     `json:"network_address_changes"`
	Messages              int     `json:"messages"`
	MessagesPerSec        float64 `json:"messages_per_sec"`
}

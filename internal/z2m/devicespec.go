package z2m

// ExposePreset is a named shortcut value for a numeric expose. Value is kept
// as any because z2m publishes both string and numeric presets.
type ExposePreset struct {
	Name        string `json:"name"`
	Value       any    `json:"value"`
	Description string `json:"description"`
}

// Expose describes a device capability, following
// https://www.zigbee2mqtt.io/guide/usage/exposes.html. Composite exposes
// (switch, light, composite) nest Features; list exposes describe their items
// in ItemType. Binary on/off values are strings or booleans depending on the
// device.
type Expose struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Property    string   `json:"property"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Unit        string   `json:"unit"`
	Access      int      `json:"access"`
	ValueOn     any      `json:"value_on"`
	ValueOff    any      `json:"value_off"`
	ValueToggle any      `json:"value_toggle"`
	ValueMin    *float64 `json:"value_min"`
	ValueMax    *float64 `json:"value_max"`
	ValueStep   *float64 `json:"value_step"`
	Values      []string `json:"values"`
	// Presets lists named shortcut values for a numeric expose
	// (e.g.  "minimum"/"previous" for a startup level)
	Presets []ExposePreset `json:"presets"`
	// Features holds the sub-features of a composite expose (switch, light, composite)
	Features []Expose `json:"features"`
	// ItemType describes the schema of each element of a "list" expose
	ItemType *Expose `json:"item_type"`
}

// DeviceDefinition describes the device model and what it exposes
type DeviceDefinition struct {
	Source      string   `json:"source"`
	Model       string   `json:"model"`
	Vendor      string   `json:"vendor"`
	Description string   `json:"description"`
	Exposes     []Expose `json:"exposes"`
}

// DeviceSpec represents a single entry from the `zigbee2mqtt/bridge/devices`
// topic. It contains the static metadata Zigbee2MQTT knows about a device.
type DeviceSpec struct {
	IEEEAddress    string            `json:"ieee_address"`
	Type           string            `json:"type"` // Coordinator | Router | EndDevice
	NetworkAddress int               `json:"network_address"`
	Supported      bool              `json:"supported"`
	Disabled       bool              `json:"disabled"`
	FriendlyName   string            `json:"friendly_name"`
	Description    string            `json:"description"`
	PowerSource    string            `json:"power_source"`
	DateCode       string            `json:"date_code"`
	ModelID        string            `json:"model_id"`
	InterviewState string            `json:"interview_state"`
	Definition     *DeviceDefinition `json:"definition"`
}

package z2m

// DeviceOptions holds the z2m device options (configuration.yaml devices and
// device_options) that shape a device's published state. Decoded from
// bridge/info, it holds the options as published; from CompileDeviceOptions,
// the options z2m applies to a device, with its filter compiled.
type DeviceOptions struct {
	// FilteredAttributes lists patterns of props z2m drops from every
	// publish (see NewPropFilter). Nil means the option is unset.
	FilteredAttributes []string `json:"filtered_attributes"`

	// propFilter is FilteredAttributes compiled; nil unless set by
	// CompileDeviceOptions.
	propFilter PropFilter
}

// CompileDeviceOptions returns the options z2m applies to a device with
// options own, given defaults (device_options, already compiled, or nil).
// z2m merges them shallowly, so own's filtered_attributes, even an empty
// list, replace the default list. Without a list of its own the device gets
// defaults itself, shared rather than copied; otherwise its list is compiled
// into new options, and the error is NewPropFilter's.
func CompileDeviceOptions(own DeviceOptions, defaults *DeviceOptions) (*DeviceOptions, error) {
	if own.FilteredAttributes == nil {
		return defaults, nil
	}
	filter, err := NewPropFilter(own.FilteredAttributes)
	return &DeviceOptions{FilteredAttributes: own.FilteredAttributes, propFilter: filter}, err
}

// filters reports whether z2m drops prop from every publish. Nil options, and
// options without a compiled filter, filter nothing.
func (opts *DeviceOptions) filters(prop string) bool {
	return opts != nil && opts.propFilter.Match(prop)
}

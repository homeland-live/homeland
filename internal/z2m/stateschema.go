package z2m

// accessPublished is bit 1 of an expose's access bitmask: the property appears
// in the device's published state. The full 3-bit mask (see
// https://www.zigbee2mqtt.io/guide/usage/exposes.html#access) is:
//
//	bit 1 (1): property appears in the device's published state
//	bit 2 (2): property can be written with a /set command
//	bit 3 (4): property can be retrieved with a /get command
const accessPublished = 1 << 0 // 1

// propAction is z2m's property for momentary button/remote events.
const propAction = "action"

// StateSchema splits a device's published state properties by z2m expose
// category: primary ones have no category (regular usage such as state or
// sensor readings), secondary ones are config or diagnostic (battery,
// linkquality, ...). Each maps a property name to its expose type.
type StateSchema struct {
	PrimaryProps   map[string]string `json:"primaryProps"`
	SecondaryProps map[string]string `json:"secondaryProps"`
}

// addProp records e as a top-level key once, under the first expose
// declaring it, when it has a property and is published (access bit 1). Its
// features are nested under it in state (level_config: {...}), not keys of
// their own. A prop filter matches is never published, so it is skipped.
// schema must come from NewStateSchema: addProp writes to its maps.
func (schema *StateSchema) addProp(e Expose, filter PropFilter) {
	if e.Property == "" || e.Access&accessPublished == 0 || schema.hasProp(e.Property) || filter.Match(e.Property) {
		return
	}
	if e.Category == "" || e.Property == propAction {
		schema.PrimaryProps[e.Property] = e.Type
	} else {
		schema.SecondaryProps[e.Property] = e.Type
	}
}

// hasProp reports whether prop is in either bucket.
func (schema *StateSchema) hasProp(prop string) bool {
	_, primary := schema.PrimaryProps[prop]
	_, secondary := schema.SecondaryProps[prop]
	return primary || secondary
}

// NewStateSchema derives the schema from the top-level state keys a
// definition publishes (access bit 1): exposes with a property, and the
// features of grouping exposes (light, switch, ...), which z2m publishes at
// top level. It looks one grouping level deep, which is all z2m produces.
// "action" is always primary. Props z2m adds outside the exposes, such as
// last_seen, are not included, and neither are the props filter matches,
// which z2m keeps out of every publish. A nil definition yields nil.
func NewStateSchema(devdef *DeviceDefinition, filter PropFilter) *StateSchema {
	if devdef == nil {
		return nil
	}
	schema := &StateSchema{
		PrimaryProps:   make(map[string]string),
		SecondaryProps: make(map[string]string),
	}
	for _, e := range devdef.Exposes {
		if e.Property != "" {
			schema.addProp(e, filter)
			continue
		}
		// A grouping expose has no key of its own; its features are
		// published at top level.
		for _, f := range e.Features {
			schema.addProp(f, filter)
		}
	}
	return schema
}

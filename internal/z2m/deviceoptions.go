package z2m

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// DeviceOptions holds the z2m device options (configuration.yaml devices and
// device_options) that shape a device's published state.
type DeviceOptions struct {
	// FilteredAttributes lists patterns of props z2m drops from every
	// publish (see NewPropFilter). Nil means the option is unset.
	FilteredAttributes []string `json:"filtered_attributes"`
}

// PropFilter matches the props z2m's filtered_attributes drop from every
// publish. A nil filter matches nothing.
type PropFilter []*regexp.Regexp

// NewPropFilter compiles patterns the way z2m tests them, as "^<pattern>$".
// A pattern Go's regexp can't compile (a JS-only construct such as a
// lookahead) is skipped, so it matches nothing; the error then names every
// skipped pattern.
func NewPropFilter(patterns []string) (PropFilter, error) {
	var filter PropFilter
	var errs []error
	for _, p := range patterns {
		re, err := regexp.Compile("^" + p + "$")
		if err != nil {
			errs = append(errs, fmt.Errorf("pattern %q: %w", p, err))
			continue
		}
		filter = append(filter, re)
	}
	if len(errs) > 0 {
		return filter, fmt.Errorf("z2m: compile filtered_attributes: %w", errors.Join(errs...))
	}
	return filter, nil
}

// Match reports whether z2m drops prop from every publish.
func (f PropFilter) Match(prop string) bool {
	return slices.ContainsFunc(f, func(re *regexp.Regexp) bool {
		return re.MatchString(prop)
	})
}

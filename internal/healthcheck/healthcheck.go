// Package healthcheck builds the health tree served by the API: named checks
// that nest, so a client can see exactly which component failed.
package healthcheck

import "strings"

// HealthCheck is a named health node with optional child checks.
type HealthCheck struct {
	Name      string        `json:"name"`
	IsHealthy bool          `json:"isHealthy"`
	Error     string        `json:"error,omitempty"`
	Checks    []HealthCheck `json:"checks,omitempty"`
}

// New builds a terminal check from an error: a nil error is healthy,
// otherwise the error message is reported.
func New(name string, err error) HealthCheck {
	hc := HealthCheck{Name: name, IsHealthy: err == nil}
	if err != nil {
		hc.Error = err.Error()
	}
	return hc
}

// Compose builds a parent check from child checks. The parent is healthy only
// when every child is; when unhealthy its Error names the failing children.
func Compose(name string, checks ...HealthCheck) HealthCheck {
	hc := HealthCheck{Name: name, IsHealthy: true, Checks: checks}
	failed := make([]string, 0, len(checks))
	for _, c := range checks {
		if !c.IsHealthy {
			hc.IsHealthy = false
			failed = append(failed, c.Name)
		}
	}
	if !hc.IsHealthy {
		hc.Error = "unhealthy: " + strings.Join(failed, ", ")
	}
	return hc
}

package namespace

import "encoding/json"

// Configuration is the administrative, revisioned view of namespace settings.
type Configuration struct {
	Namespace  string                          `json:"namespace"`
	Generation int64                           `json:"generation"`
	Defaults   map[string]ConfigurationDefault `json:"defaults"`
	Groups     map[string]ConfigurationGroup   `json:"groups"`
}

// ConfigurationGroup is independently versioned; Values contains only owned fields.
type ConfigurationGroup struct {
	Revision int64                      `json:"revision"`
	Values   map[string]json.RawMessage `json:"values"`
	Locks    map[string]string          `json:"locks"`
	Guidance string                     `json:"guidance"`
}

// ConfigurationPatch carries a lifecycle and group precondition.
type ConfigurationPatch struct {
	Group        string                     `json:"group"`
	Generation   int64                      `json:"generation"`
	BaseRevision int64                      `json:"base_revision"`
	Changes      map[string]json.RawMessage `json:"changes"`
}

// ConfigurationError exposes safe validation or conflict details to operators.
type ConfigurationError struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Current *Configuration    `json:"current,omitempty"`
}

func (e *ConfigurationError) Error() string { return e.Message }

// ConfigurationDefault describes observations from authoritative process reports.
// Unknown or mixed observations never become an invented effective value.
type ConfigurationDefault struct {
	State   string                       `json:"state"`
	Process string                       `json:"process"`
	Value   any                          `json:"value,omitempty"`
	Reports []ConfigurationDefaultReport `json:"reports"`
}

// ConfigurationDefaultReport is one process instance's view of an inherited default.
type ConfigurationDefaultReport struct {
	Instance   string `json:"instance"`
	ReportedAt string `json:"reported_at"`
	Value      any    `json:"value"`
}

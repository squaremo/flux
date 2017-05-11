package service

type Status struct {
	Fluxsvc FluxsvcStatus `json:"fluxsvc" yaml:"fluxsvc"`
	Fluxd   FluxdStatus   `json:"fluxd" yaml:"fluxd"`
	Git     GitStatus     `json:"git" yaml:"git"`
}

type FluxsvcStatus struct {
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

type FluxdStatus struct {
	Connected bool   `json:"connected" yaml:"connected"`
	Version   string `json:"version,omitempty" yaml:"version,omitempty"`
}

type GitStatus struct {
	Configured bool   `json:"configured" yaml:"configured"`
	Error      string `json:"error,omitempty" yaml:"error,omitempty"`
}

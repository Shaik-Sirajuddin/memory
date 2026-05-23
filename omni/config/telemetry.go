package config

// OtelConfig holds OpenTelemetry export settings for omni's own logs and traces.
// All fields are optional — omni runs without telemetry when this section is absent.
type OtelConfig struct {
	Endpoint    *string           `json:"endpoint,omitempty"    jsonschema:"title=Endpoint,description=OTLP gRPC or HTTP endpoint (e.g. http://localhost:4317)"`
	Environment *string           `json:"environment,omitempty" jsonschema:"title=Environment,description=Deployment environment tag (e.g. dev / staging / prod)"`
	LogPrompts  *bool             `json:"log_prompts,omitempty" jsonschema:"title=Log Prompts,description=Include user prompt text in trace spans"`
	Headers     map[string]string `json:"headers,omitempty"     jsonschema:"title=Headers,description=Additional OTLP export headers (e.g. Authorization: Bearer token)"`
}

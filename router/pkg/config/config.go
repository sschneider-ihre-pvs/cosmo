package config

import (
	b64 "encoding/base64"
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"os"
)

type Base64Decoder []byte

func (ipd *Base64Decoder) Decode(value string) error {
	decoded, err := b64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("could not decode base64 string: %w", err)
	}

	*ipd = decoded

	return nil
}

type Graph struct {
	Name  string `yaml:"name" envconfig:"FEDERATED_GRAPH_NAME" validate:"required"`
	Token string `yaml:"token" envconfig:"GRAPH_API_TOKEN" validate:"required"`
}

type Tracing struct {
	Enabled bool `yaml:"enabled" default:"true" envconfig:"TRACING_ENABLED"`
	Config  struct {
		BatchTimeoutSeconds int     `yaml:"batch_timeout_seconds" default:"10" envconfig:"TRACING_BATCH_TIMEOUT_SECONDS"`
		SamplingRate        float64 `yaml:"sampling_rate" default:"1" validate:"min=0,max=1" envconfig:"TRACING_SAMPLING_RATE"`
	} `yaml:"config"`
}

type Prometheus struct {
	Enabled    bool   `yaml:"enabled" default:"true" envconfig:"PROMETHEUS_ENABLED"`
	Path       string `yaml:"path" default:"/metrics" validate:"uri" envconfig:"PROMETHEUS_HTTP_PATH"`
	ListenAddr string `yaml:"listen_addr" default:"127.0.0.1:8088" validate:"hostname_port" envconfig:"PROMETHEUS_LISTEN_ADDR"`
}

type Metrics struct {
	Common     MetricsCommon `yaml:"common"`
	Prometheus Prometheus    `yaml:"prometheus"`
}

type MetricsCommon struct {
	Enabled bool `yaml:"enabled" default:"true" envconfig:"METRICS_ENABLED"`
}

type OpenTelemetry struct {
	ServiceName string            `yaml:"service_name" default:"cosmo-router" envconfig:"TELEMETRY_SERVICE_NAME" validate:"required"`
	Endpoint    string            `yaml:"endpoint" validate:"required" default:"https://cosmo-otel.wundergraph.com" envconfig:"TELEMETRY_ENDPOINT" validate:"http_url"`
	Headers     map[string]string `yaml:"headers" envconfig:"TELEMETRY_HEADERS"`

	Tracing Tracing `yaml:"tracing"`
	Metrics Metrics `yaml:"metrics"`
}

type CORS struct {
	AllowOrigins     []string `yaml:"allow_origins" default:"*" envconfig:"CORS_ALLOW_ORIGINS"`
	AllowMethods     []string `yaml:"allow_methods" default:"HEAD,GET,POST" envconfig:"CORS_ALLOW_METHODS"`
	AllowHeaders     []string `yaml:"allow_headers" default:"Origin,Content-Length,Content-Type" envconfig:"CORS_ALLOW_HEADERS"`
	AllowCredentials bool     `yaml:"allow_credentials" default:"true" envconfig:"CORS_ALLOW_CREDENTIALS"`
	MaxAgeMinutes    int      `yaml:"max_age_minutes" default:"5" validate:"min=5" envconfig:"CORS_MAX_AGE_MINUTES"`
}

type Config struct {
	Version string `yaml:"version"`

	Graph     Graph         `yaml:"graph"`
	Telemetry OpenTelemetry `yaml:"telemetry"`
	CORS      CORS          `yaml:"cors"`

	Modules map[string]interface{} `yaml:"modules"`

	ListenAddr           string `yaml:"listen_addr" default:"localhost:3002" validate:"hostname_port" envconfig:"LISTEN_ADDR"`
	ControlplaneURL      string `yaml:"controlplane_url" validate:"required" default:"https://cosmo-cp.wundergraph.com" envconfig:"CONTROLPLANE_URL" validate:"uri"`
	PlaygroundEnabled    bool   `yaml:"playground_enabled" default:"true" envconfig:"PLAYGROUND_ENABLED"`
	IntrospectionEnabled bool   `yaml:"introspection_enabled" default:"true" envconfig:"INTROSPECTION_ENABLED"`
	LogLevel             string `yaml:"log_level" default:"info" envconfig:"LOG_LEVEL" validate:"oneof=debug info warning error fatal panic"`
	JSONLog              bool   `yaml:"json_log" default:"true" envconfig:"JSON_LOG"`
	ShutdownDelaySeconds int    `yaml:"shutdown_delay_seconds" default:"15" validate:"min=5" envconfig:"SHUTDOWN_DELAY_SECONDS"`
	GracePeriodSeconds   int    `yaml:"grace_period_seconds" default:"0" envconfig:"GRACE_PERIOD_SECONDS"`
	PollIntervalSeconds  int    `yaml:"poll_interval_seconds" default:"10" validate:"min=5" envconfig:"POLL_INTERVAL_SECONDS"`
	HealthCheckPath      string `yaml:"health_check_path" default:"/health" envconfig:"HEALTH_CHECK_PATH" validate:"uri"`

	ConfigPath       string `default:"config.yaml" envconfig:"CONFIG_PATH" validate:"omitempty,filepath"`
	RouterConfigPath string `yaml:"router_config_path" envconfig:"ROUTER_CONFIG_PATH" validate:"omitempty,filepath"`
}

func LoadConfig() (*Config, error) {
	godotenv.Load(".env.local")
	godotenv.Load()

	var c Config

	err := envconfig.Process("", &c)
	if err != nil {
		return nil, err
	}

	configBytes, err := os.ReadFile(c.ConfigPath)

	if err == nil {
		if err := yaml.Unmarshal(configBytes, &c); err != nil {
			return nil, fmt.Errorf("failed to unmarshal router config: %w", err)
		}
	}

	err = validator.New().Struct(c)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

package serve

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the top-level configuration for the serve command.
type Config struct {
	Server    ServerConfig     `toml:"server"`
	Inverters []InverterConfig `toml:"inverter"`
	MQTT      *MQTTConfig      `toml:"mqtt"`
}

// ServerConfig controls the polling loop and web server.
type ServerConfig struct {
	PollInterval    string `toml:"poll_interval"`
	WebPort         int    `toml:"web_port"`
	SettingsRefresh string `toml:"settings_refresh"`
}

// InverterConfig identifies a single Solarman dongle.
type InverterConfig struct {
	Driver     string `toml:"driver"`
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	Serial     uint32 `toml:"serial"`
	SlaveID    uint8  `toml:"slave_id"`
	MPPT1Label string `toml:"mppt1_label"`
	MPPT2Label string `toml:"mppt2_label"`
}

// MQTTConfig controls the MQTT publisher. Optional — nil when omitted.
type MQTTConfig struct {
	Broker      string `toml:"broker"`
	ClientID    string `toml:"client_id"`
	Username    string `toml:"username"`
	Password    string `toml:"password"`
	TopicPrefix string `toml:"topic_prefix"`
}

// Defaults.
const (
	defaultPollInterval    = 5 * time.Second
	defaultSettingsRefresh = 5 * time.Minute
	defaultWebPort         = 8080
	defaultSolarmanPort    = 8899
	defaultMQTTClientID    = "srne"
	defaultTopicPrefix     = "homeassistant"
)

// LoadConfig reads and validates a TOML config file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if len(cfg.Inverters) == 0 {
		return nil, fmt.Errorf("config: at least one [[inverter]] is required")
	}

	// Apply defaults
	if cfg.Server.WebPort == 0 {
		cfg.Server.WebPort = defaultWebPort
	}
	if cfg.Server.PollInterval == "" {
		cfg.Server.PollInterval = defaultPollInterval.String()
	}
	if cfg.Server.SettingsRefresh == "" {
		cfg.Server.SettingsRefresh = defaultSettingsRefresh.String()
	}

	for i := range cfg.Inverters {
		if cfg.Inverters[i].Driver == "" {
			cfg.Inverters[i].Driver = "solarman"
		}

		switch cfg.Inverters[i].Driver {
		case "solarman":
			if cfg.Inverters[i].Host == "" {
				return nil, fmt.Errorf("config: inverter[%d] missing host", i)
			}
			if cfg.Inverters[i].Port == 0 {
				cfg.Inverters[i].Port = defaultSolarmanPort
			}
		case "mock":
			if cfg.Inverters[i].Host == "" {
				cfg.Inverters[i].Host = fmt.Sprintf("mock-%d", i+1)
			}
		default:
			return nil, fmt.Errorf("config: inverter[%d] invalid driver %q", i, cfg.Inverters[i].Driver)
		}

		if cfg.Inverters[i].Driver == "solarman" && cfg.Inverters[i].Host == "" {
			return nil, fmt.Errorf("config: inverter[%d] missing host", i)
		}
	}

	if cfg.MQTT != nil {
		if cfg.MQTT.Broker == "" {
			return nil, fmt.Errorf("config: mqtt.broker is required when [mqtt] is present")
		}
		if cfg.MQTT.ClientID == "" {
			cfg.MQTT.ClientID = defaultMQTTClientID
		}
		if cfg.MQTT.TopicPrefix == "" {
			cfg.MQTT.TopicPrefix = defaultTopicPrefix
		}
	}

	// Validate durations parse
	if _, err := cfg.PollIntervalDuration(); err != nil {
		return nil, fmt.Errorf("config: poll_interval: %w", err)
	}
	if _, err := cfg.SettingsRefreshDuration(); err != nil {
		return nil, fmt.Errorf("config: settings_refresh: %w", err)
	}

	return &cfg, nil
}

// PollIntervalDuration parses the poll_interval string.
func (c *Config) PollIntervalDuration() (time.Duration, error) {
	return time.ParseDuration(c.Server.PollInterval)
}

// SettingsRefreshDuration parses the settings_refresh string.
func (c *Config) SettingsRefreshDuration() (time.Duration, error) {
	return time.ParseDuration(c.Server.SettingsRefresh)
}

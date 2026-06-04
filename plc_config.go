package main

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultConnectTimeout = 8 * time.Second
	defaultPollInterval   = 10 * time.Second
)

type plcConfig struct {
	ConnectionString string
	Tags             []plcTag
	Timeout          time.Duration
	PollInterval     time.Duration
}

type plcTag struct {
	Name     string
	Address  string
	Writable bool
}

func parsePLCConfig(cfg map[string]any) (plcConfig, error) {
	out := plcConfig{
		ConnectionString: configString(cfg, "connection_string", envAny("PLC4X_CONNECTION_STRING")),
		Timeout:          time.Duration(configInt(cfg, "connect_timeout_seconds", envInt("PLC4X_CONNECT_TIMEOUT_SECONDS", 8))) * time.Second,
		PollInterval:     time.Duration(configInt(cfg, "poll_interval_seconds", envInt("PLC4X_TELEMETRY_INTERVAL_SECONDS", 10))) * time.Second,
	}
	if out.Timeout <= 0 {
		out.Timeout = defaultConnectTimeout
	}
	if out.PollInterval <= 0 {
		out.PollInterval = defaultPollInterval
	}
	if out.ConnectionString == "" {
		return plcConfig{}, fmt.Errorf("connection_string is required")
	}
	tags, err := parsePLCTags(cfg["tags"])
	if err != nil {
		return plcConfig{}, err
	}
	out.Tags = tags
	return out, nil
}

func parsePLCTags(raw any) ([]plcTag, error) {
	rawItems, ok := raw.([]any)
	if !ok || len(rawItems) == 0 {
		return nil, fmt.Errorf("at least one plc tag is required")
	}
	tags := make([]plcTag, 0, len(rawItems))
	seen := map[string]struct{}{}
	for _, item := range rawItems {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("plc tag must be an object")
		}
		tag := plcTag{
			Name:     configString(m, "name", ""),
			Address:  configString(m, "address", ""),
			Writable: configBool(m, "writable", false),
		}
		if tag.Name == "" || tag.Address == "" {
			return nil, fmt.Errorf("plc tag name and address are required")
		}
		if _, exists := seen[tag.Name]; exists {
			return nil, fmt.Errorf("duplicate plc tag name %q", tag.Name)
		}
		seen[tag.Name] = struct{}{}
		tags = append(tags, tag)
	}
	return tags, nil
}

func (cfg plcConfig) TagByName(name string) (plcTag, bool) {
	for _, tag := range cfg.Tags {
		if tag.Name == name {
			return tag, true
		}
	}
	return plcTag{}, false
}

func configBool(cfg map[string]any, key string, fallback bool) bool {
	switch value := cfg[key].(type) {
	case bool:
		return value
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	case float64:
		return value != 0
	}
	return fallback
}

func sanitizeConnectionString(connectionString string) string {
	if i := strings.Index(connectionString, "?"); i >= 0 {
		return connectionString[:i] + "?..."
	}
	return connectionString
}

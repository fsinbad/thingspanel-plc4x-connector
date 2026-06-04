package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	sdk "github.com/thingspanel/device-connector-sdk-go"
)

type plcTelemetryConfig struct {
	Broker   string
	Password string
	Interval time.Duration
}

var telemetryMQTTClients = struct {
	mu      sync.Mutex
	clients map[string]mqtt.Client
}{
	clients: map[string]mqtt.Client{},
}

func startPLCTelemetry(ctx context.Context, handler *handler) {
	cfg := plcTelemetryConfig{
		Broker:   strings.TrimSpace(envAny("TP_MQTT_BROKER", "MQTT_BROKER")),
		Password: envAny("PLC4X_MQTT_PASSWORD"),
		Interval: time.Duration(envInt("PLC4X_TELEMETRY_INTERVAL_SECONDS", 10)) * time.Second,
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultPollInterval
	}
	if cfg.Broker == "" {
		slog.Info("plc telemetry disabled: missing TP_MQTT_BROKER")
		return
	}
	go runPLCTelemetry(ctx, handler, cfg)
}

func runPLCTelemetry(ctx context.Context, handler *handler, cfg plcTelemetryConfig) {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		publishPLCTelemetry(ctx, handler, cfg)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func publishPLCTelemetry(ctx context.Context, handler *handler, cfg plcTelemetryConfig) {
	for _, device := range handler.boundDevices() {
		publishBoundDeviceTelemetry(ctx, handler, cfg, device)
	}
}

func publishBoundDeviceTelemetry(ctx context.Context, handler *handler, cfg plcTelemetryConfig, device sdk.DeviceAddRequest) {
	accessToken := strings.TrimSpace(device.AccessToken)
	if accessToken == "" {
		accessToken = deviceNumberFromRequest(device)
	}
	if accessToken == "" {
		return
	}
	client, err := newDeviceMQTTClient(cfg, device.DeviceID, accessToken)
	if err != nil {
		slog.Warn("plc telemetry MQTT connect failed", "deviceID", device.DeviceID, "err", err)
		return
	}

	deviceCfg := handler.configFor(device.DeviceID)
	plcCfg, err := parsePLCConfig(deviceCfg)
	if err != nil {
		slog.Warn("plc telemetry config invalid", "deviceID", device.DeviceID, "err", err)
		publishStatus(client, device.DeviceID, false)
		return
	}

	values, err := handler.client.ReadTags(ctx, plcCfg)
	if err != nil {
		slog.Warn("plc read failed", "deviceID", device.DeviceID, "connection", sanitizeConnectionString(plcCfg.ConnectionString), "err", err)
		publishStatus(client, device.DeviceID, false)
		return
	}
	publishStatus(client, device.DeviceID, true)
	values["plc_online"] = true
	values["plc_source"] = "plc4x-service"
	values["plc_observed_at"] = time.Now().UTC().Format(time.RFC3339)

	raw, err := json.Marshal(values)
	if err != nil {
		slog.Warn("marshal plc telemetry failed", "deviceID", device.DeviceID, "err", err)
		return
	}
	token := client.Publish("devices/telemetry", 0, false, raw)
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		slog.Warn("publish plc telemetry failed", "deviceID", device.DeviceID, "err", token.Error())
		return
	}
	slog.Info("plc telemetry published", "deviceID", device.DeviceID, "tagCount", len(plcCfg.Tags))
}

func publishStatus(client mqtt.Client, deviceID string, online bool) {
	payload := "0"
	if online {
		payload = "1"
	}
	token := client.Publish("devices/status/"+deviceID, 0, false, []byte(payload))
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		slog.Warn("publish plc status failed", "deviceID", deviceID, "err", token.Error())
	}
}

func newDeviceMQTTClient(cfg plcTelemetryConfig, deviceID string, accessToken string) (mqtt.Client, error) {
	clientKey := deviceID + ":" + accessToken

	telemetryMQTTClients.mu.Lock()
	if existing, ok := telemetryMQTTClients.clients[clientKey]; ok && existing != nil && existing.IsConnected() {
		telemetryMQTTClients.mu.Unlock()
		return existing, nil
	}
	telemetryMQTTClients.mu.Unlock()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.Broker)
	opts.SetClientID("plc4x-telemetry-" + deviceID)
	opts.SetUsername(accessToken)
	if cfg.Password != "" {
		opts.SetPassword(cfg.Password)
	}
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(5 * time.Second) {
		return nil, context.DeadlineExceeded
	}
	if err := token.Error(); err != nil {
		return nil, err
	}

	telemetryMQTTClients.mu.Lock()
	telemetryMQTTClients.clients[clientKey] = client
	telemetryMQTTClients.mu.Unlock()

	return client, nil
}

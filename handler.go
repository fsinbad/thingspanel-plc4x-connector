package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	sdk "github.com/thingspanel/device-connector-sdk-go"
)

type handler struct {
	logger          *slog.Logger
	client          *plcClient
	mu              sync.RWMutex
	devices         map[string]sdk.DeviceAddRequest
	deviceNumberIDs map[string]string
}

func newHandler() *handler {
	return &handler{
		logger:          slog.Default(),
		client:          newPLCClient(),
		devices:         map[string]sdk.DeviceAddRequest{},
		deviceNumberIDs: map[string]string{},
	}
}

func (h *handler) FormConfigFor(_ context.Context, req sdk.FormConfigRequest) (sdk.FormConfig, error) {
	if strings.EqualFold(req.FormType, "CFG") {
		return sdk.FormConfig{Schema: deviceConfigSchema()}, nil
	}
	return h.FormConfig(context.Background())
}

func (h *handler) FormConfig(context.Context) (sdk.FormConfig, error) {
	return sdk.FormConfig{Schema: map[string]any{
		"type":        "object",
		"title":       "PLC4X 接入服务",
		"description": "通过 Apache PLC4Go 连接 PLC，按配置的 tag 轮询读数并上报 ThingsPanel。",
		"properties": map[string]any{
			"connection_string": map[string]any{
				"type":        "string",
				"title":       "PLC4X 连接串",
				"description": "例如 modbus-tcp://192.168.1.10:502?unit-identifier=1、s7://192.168.1.20、opcua://host:4840/path",
			},
			"poll_interval_seconds": map[string]any{
				"type":        "integer",
				"title":       "轮询间隔秒数",
				"description": "不填则使用 PLC4X_TELEMETRY_INTERVAL_SECONDS，默认 10 秒。",
				"default":     10,
				"minimum":     1,
			},
			"connect_timeout_seconds": map[string]any{
				"type":    "integer",
				"title":   "连接/读写超时秒数",
				"default": 8,
				"minimum": 1,
			},
			"tags": tagsSchema("默认采集点位"),
			"devices": map[string]any{
				"type":        "array",
				"title":       "可选：预置导入设备",
				"description": "PLC 通常无法通用发现设备；这里可预置多个 ThingsPanel 子设备，每个设备可覆盖 connection_string/tags。",
				"items":       discoveredDeviceSchema(),
			},
		},
		"required": []string{"connection_string"},
		"x-order":  []string{"connection_string", "poll_interval_seconds", "connect_timeout_seconds", "tags", "devices"},
	}}, nil
}

func deviceConfigSchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"title":       "PLC 设备点位配置",
		"description": "单个 ThingsPanel 设备绑定到一组 PLC tag。connection_string 为空时继承接入服务配置。",
		"properties": map[string]any{
			"connection_string": map[string]any{
				"type":        "string",
				"title":       "PLC4X 连接串",
				"description": "可选，覆盖接入服务上的连接串。",
			},
			"poll_interval_seconds": map[string]any{
				"type":    "integer",
				"title":   "轮询间隔秒数",
				"minimum": 1,
			},
			"connect_timeout_seconds": map[string]any{
				"type":    "integer",
				"title":   "连接/读写超时秒数",
				"minimum": 1,
			},
			"tags": tagsSchema("采集/写入点位"),
		},
		"required": []string{"tags"},
	}
}

func tagsSchema(title string) map[string]any {
	return map[string]any{
		"type":  "array",
		"title": title,
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"title":       "字段名",
					"description": "上报到 ThingsPanel telemetry 的字段名，例如 temperature。",
				},
				"address": map[string]any{
					"type":        "string",
					"title":       "PLC4X tag 地址",
					"description": "例如 holding-register:1:REAL。不同协议按 PLC4X 对应地址格式填写。",
				},
				"writable": map[string]any{
					"type":        "boolean",
					"title":       "允许写入",
					"description": "开启后才允许 ThingsPanel 下行 write 到该 tag。",
					"default":     false,
				},
			},
			"required": []string{"name", "address"},
		},
	}
}

func discoveredDeviceSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"device_name":       map[string]any{"type": "string", "title": "设备名称"},
			"device_number":     map[string]any{"type": "string", "title": "设备编号"},
			"description":       map[string]any{"type": "string", "title": "描述"},
			"connection_string": map[string]any{"type": "string", "title": "PLC4X 连接串"},
			"tags":              tagsSchema("设备点位"),
		},
		"required": []string{"device_name", "device_number", "tags"},
	}
}

func (h *handler) ListDevices(_ context.Context, req sdk.DeviceListRequest) (sdk.DeviceListResponse, error) {
	cfg, err := parseVoucher(req.Voucher)
	if err != nil {
		return sdk.DeviceListResponse{}, err
	}
	devices := configuredDevices(cfg)
	if len(devices) == 0 {
		connectionString := configString(cfg, "connection_string", envAny("PLC4X_CONNECTION_STRING"))
		if connectionString == "" {
			return sdk.DeviceListResponse{List: []sdk.DiscoveredDevice{}}, nil
		}
		devices = []sdk.DiscoveredDevice{{
			DeviceName:     "PLC Gateway",
			DeviceNumber:   normalizedDeviceNumber("plc4x", connectionString),
			Description:    "PLC4X connection: " + sanitizeConnectionString(connectionString),
			ProtocolConfig: jsonString(cfg),
			AdditionalInfo: jsonString(map[string]any{"source": "plc4x-service", "mode": "manual"}),
		}}
	}
	return paginateDevices(devices, req.Page, req.PageSize), nil
}

func configuredDevices(cfg map[string]any) []sdk.DiscoveredDevice {
	rawDevices, ok := cfg["devices"].([]any)
	if !ok || len(rawDevices) == 0 {
		return nil
	}
	devices := make([]sdk.DiscoveredDevice, 0, len(rawDevices))
	for i, raw := range rawDevices {
		device, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := configString(device, "device_name", fmt.Sprintf("PLC Device %d", i+1))
		number := configString(device, "device_number", normalizedDeviceNumber("plc4x", name))
		protocolConfig := mergeMaps(cfg, device)
		delete(protocolConfig, "devices")
		devices = append(devices, sdk.DiscoveredDevice{
			DeviceName:     name,
			DeviceNumber:   number,
			Description:    configString(device, "description", "PLC4X configured device"),
			ProtocolConfig: jsonString(protocolConfig),
			AdditionalInfo: jsonString(map[string]any{"source": "plc4x-service", "mode": "configured"}),
		})
	}
	return devices
}

func (h *handler) OnDeviceAdd(_ context.Context, req sdk.DeviceAddRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.devices[req.DeviceID] = req
	if deviceNumber := deviceNumberFromRequest(req); deviceNumber != "" {
		h.deviceNumberIDs[deviceNumber] = req.DeviceID
	}
	h.logger.Info("plc device added", "deviceID", req.DeviceID, "deviceNumber", deviceNumberFromRequest(req))
	return nil
}

func (h *handler) OnDeviceDelete(_ context.Context, req sdk.DeviceDeleteRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.devices[req.DeviceID]; ok {
		delete(h.deviceNumberIDs, deviceNumberFromRequest(existing))
	}
	delete(h.devices, req.DeviceID)
	return nil
}

func (h *handler) OnConfigUpdate(_ context.Context, req sdk.ConfigUpdateRequest) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.devices[req.DeviceID]; ok {
		existing.DeviceConfig = req.DeviceConfig
		h.devices[req.DeviceID] = existing
	}
	return nil
}

func (h *handler) OnCommand(ctx context.Context, req sdk.CommandRequest) (sdk.CommandResponse, error) {
	cfg := h.configFor(req.DeviceID)
	plcCfg, err := parsePLCConfig(cfg)
	if err != nil {
		return sdk.CommandResponse{}, err
	}
	if len(req.Command) == 0 {
		return sdk.CommandResponse{}, fmt.Errorf("command payload is required")
	}
	if hasCommand(req.Command, "query") {
		values, err := h.client.ReadTags(ctx, plcCfg)
		if err != nil {
			return sdk.CommandResponse{}, err
		}
		raw, _ := json.Marshal(values)
		return sdk.CommandResponse{OK: true, Message: string(raw)}, nil
	}
	tagName, value, err := parseWriteCommand(req.Command)
	if err != nil {
		return sdk.CommandResponse{}, err
	}
	tag, ok := plcCfg.TagByName(tagName)
	if !ok {
		return sdk.CommandResponse{}, fmt.Errorf("unknown plc tag %q", tagName)
	}
	if !tag.Writable {
		return sdk.CommandResponse{}, fmt.Errorf("plc tag %q is not marked writable", tagName)
	}
	if err := h.client.WriteTag(ctx, plcCfg, tag, value); err != nil {
		return sdk.CommandResponse{}, err
	}
	return sdk.CommandResponse{OK: true, Message: fmt.Sprintf("plc tag %s written", tagName)}, nil
}

func (h *handler) OnDisconnect(context.Context, sdk.DisconnectRequest) error {
	return nil
}

func (h *handler) OnEvent(context.Context, sdk.EventNotification) error {
	return nil
}

func (h *handler) configFor(deviceID string) map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if req, ok := h.devices[deviceID]; ok && req.DeviceConfig != nil {
		return req.DeviceConfig
	}
	return map[string]any{
		"connection_string":       envAny("PLC4X_CONNECTION_STRING"),
		"tags":                    parseTagsEnv(),
		"poll_interval_seconds":   envInt("PLC4X_TELEMETRY_INTERVAL_SECONDS", 10),
		"connect_timeout_seconds": envInt("PLC4X_CONNECT_TIMEOUT_SECONDS", 8),
	}
}

func (h *handler) boundDevices() []sdk.DeviceAddRequest {
	h.mu.RLock()
	defer h.mu.RUnlock()
	devices := make([]sdk.DeviceAddRequest, 0, len(h.devices))
	for _, req := range h.devices {
		devices = append(devices, req)
	}
	return devices
}

func (h *handler) deviceIDForNumber(deviceNumber string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.deviceNumberIDs[deviceNumber]
}

func deviceNumberFromRequest(req sdk.DeviceAddRequest) string {
	if req.DeviceConfig == nil {
		return ""
	}
	return strings.TrimSpace(configString(req.DeviceConfig, "device_number", ""))
}

func parseVoucher(raw string) (map[string]any, error) {
	cfg := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("invalid voucher json: %w", err)
	}
	return cfg, nil
}

func parseWriteCommand(command map[string]any) (string, any, error) {
	if raw, ok := command["write"]; ok {
		writeMap, ok := raw.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("write command must be an object with tag and value")
		}
		tag := configString(writeMap, "tag", "")
		if tag == "" {
			tag = configString(writeMap, "name", "")
		}
		if tag == "" {
			return "", nil, fmt.Errorf("write command tag is required")
		}
		value, ok := writeMap["value"]
		if !ok {
			return "", nil, fmt.Errorf("write command value is required")
		}
		return tag, value, nil
	}
	if tag := configString(command, "tag", ""); tag != "" {
		value, ok := command["value"]
		if !ok {
			return "", nil, fmt.Errorf("command value is required")
		}
		return tag, value, nil
	}
	return "", nil, fmt.Errorf("unsupported command keys: %v", command)
}

func hasCommand(command map[string]any, key string) bool {
	_, ok := command[key]
	return ok
}

func parseTagsEnv() []any {
	raw := strings.TrimSpace(envAny("PLC4X_TAGS_JSON"))
	if raw == "" {
		return nil
	}
	var tags []any
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		slog.Warn("invalid PLC4X_TAGS_JSON", "err", err)
		return nil
	}
	return tags
}

func configString(cfg map[string]any, key string, fallback string) string {
	if cfg == nil {
		return strings.TrimSpace(fallback)
	}
	if value, ok := cfg[key]; ok {
		switch v := value.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case fmt.Stringer:
			if strings.TrimSpace(v.String()) != "" {
				return strings.TrimSpace(v.String())
			}
		}
	}
	return strings.TrimSpace(fallback)
}

func configInt(cfg map[string]any, key string, fallback int) int {
	if cfg == nil {
		return fallback
	}
	switch value := cfg[key].(type) {
	case int:
		if value > 0 {
			return value
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envAny(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func jsonString(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func mergeMaps(base map[string]any, override map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func paginateDevices(devices []sdk.DiscoveredDevice, page, pageSize int) sdk.DeviceListResponse {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	total := len(devices)
	start := (page - 1) * pageSize
	if start >= total {
		return sdk.DeviceListResponse{Total: total, List: []sdk.DiscoveredDevice{}}
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return sdk.DeviceListResponse{Total: total, List: devices[start:end]}
}

func normalizedDeviceNumber(prefix, value string) string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range cleaned {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "device"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return prefix + "-" + out
}

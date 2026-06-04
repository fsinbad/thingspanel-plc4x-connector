package main

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/thingspanel/device-connector-sdk-go"
)

func TestFormConfig(t *testing.T) {
	cfg, err := newHandler().FormConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	props := schemaProperties(t, cfg.Schema)
	for _, key := range []string{"connection_string", "poll_interval_seconds", "connect_timeout_seconds", "tags", "devices"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("%s field not found", key)
		}
	}
}

func TestListDevicesFromVoucher(t *testing.T) {
	voucher := map[string]any{
		"connection_string": "modbus-tcp://127.0.0.1:502?unit-identifier=1",
		"devices": []map[string]any{{
			"device_name":   "Boiler PLC",
			"device_number": "boiler-plc-01",
			"tags": []map[string]any{{
				"name":    "temperature",
				"address": "holding-register:1:REAL",
			}},
		}},
	}
	raw, _ := json.Marshal(voucher)
	resp, err := newHandler().ListDevices(context.Background(), sdk.DeviceListRequest{
		Voucher:  string(raw),
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || len(resp.List) != 1 {
		t.Fatalf("unexpected device list: total=%d len=%d", resp.Total, len(resp.List))
	}
	if resp.List[0].DeviceNumber != "boiler-plc-01" {
		t.Fatalf("unexpected device number %q", resp.List[0].DeviceNumber)
	}
}

func TestParsePLCConfig(t *testing.T) {
	cfg, err := parsePLCConfig(map[string]any{
		"connection_string": "modbus-tcp://127.0.0.1:502?unit-identifier=1",
		"tags": []any{
			map[string]any{"name": "temperature", "address": "holding-register:1:REAL"},
			map[string]any{"name": "pump_start", "address": "coil:1:BOOL", "writable": true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(cfg.Tags))
	}
	tag, ok := cfg.TagByName("pump_start")
	if !ok || !tag.Writable {
		t.Fatalf("expected writable pump_start tag, got %#v", tag)
	}
}

func TestParseWriteCommand(t *testing.T) {
	tag, value, err := parseWriteCommand(map[string]any{
		"write": map[string]any{"tag": "pump_start", "value": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tag != "pump_start" || value != true {
		t.Fatalf("unexpected write command tag=%s value=%v", tag, value)
	}
}

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties not found")
	}
	return props
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	sdk "github.com/thingspanel/device-connector-sdk-go"
)

type mqttCommandBridge struct {
	info    sdk.ConnectorInfo
	handler *handler
	broker  string
	logger  *slog.Logger
}

type mqttCommandPayload struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func startMQTTCommandBridge(ctx context.Context, info sdk.ConnectorInfo, handler *handler) {
	bridge, err := newMQTTCommandBridge(info, handler)
	if err != nil {
		slog.Warn("mqtt command bridge disabled", "err", err)
		return
	}
	go bridge.run(ctx)
}

func newMQTTCommandBridge(info sdk.ConnectorInfo, handler *handler) (*mqttCommandBridge, error) {
	broker := envAny("TP_MQTT_BROKER", "MQTT_BROKER")
	if broker == "" {
		return nil, fmt.Errorf("TP_MQTT_BROKER is required")
	}
	return &mqttCommandBridge{info: info, handler: handler, broker: broker, logger: slog.Default()}, nil
}

func (b *mqttCommandBridge) run(ctx context.Context) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(b.broker)
	opts.SetClientID(fmt.Sprintf("%s-plc4x-command-%d", b.info.InstanceID, time.Now().UnixNano()))
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	if user := envAny("TP_MQTT_USER", "MQTT_USER", "MQTT_USERNAME"); user != "" {
		opts.SetUsername(user)
	}
	if pass := envAny("TP_MQTT_PASS", "MQTT_PASS", "MQTT_PASSWORD"); pass != "" {
		opts.SetPassword(pass)
	}
	pluginTopic := fmt.Sprintf("plugin/%s/devices/command/+/+", b.info.ServiceIdentifier)
	nativeTopic := "devices/command/+/+"
	opts.OnConnect = func(client mqtt.Client) {
		filters := map[string]byte{pluginTopic: 1, nativeTopic: 1}
		token := client.SubscribeMultiple(filters, b.handleCommandMessage)
		token.Wait()
		if err := token.Error(); err != nil {
			b.logger.Error("subscribe failed", "topics", []string{pluginTopic, nativeTopic}, "err", err)
			return
		}
		b.logger.Info("mqtt command bridge subscribed", "topics", []string{pluginTopic, nativeTopic})
	}
	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		b.logger.Error("mqtt connect failed", "err", err)
		return
	}
	defer client.Disconnect(250)
	<-ctx.Done()
}

func (b *mqttCommandBridge) handleCommandMessage(client mqtt.Client, msg mqtt.Message) {
	messageID := messageIDFromTopic(msg.Topic())
	deviceNumber := deviceNumberFromTopic(msg.Topic())
	req, err := b.decodeCommand(deviceNumber, msg.Payload())
	if err != nil {
		b.publishResponse(client, "", messageID, err)
		return
	}
	resp, err := b.handler.OnCommand(context.Background(), req)
	if err != nil {
		b.publishResponse(client, req.DeviceID, messageID, err)
		return
	}
	b.logger.Info("plc command handled", "message", resp.Message)
	b.publishResponse(client, req.DeviceID, messageID, nil)
}

func (b *mqttCommandBridge) decodeCommand(deviceNumber string, payload []byte) (sdk.CommandRequest, error) {
	deviceID := strings.TrimSpace(b.handler.deviceIDForNumber(deviceNumber))
	if deviceID == "" {
		return sdk.CommandRequest{}, fmt.Errorf("device number %s is not bound in connector", deviceNumber)
	}
	var in mqttCommandPayload
	if err := json.Unmarshal(payload, &in); err != nil {
		return sdk.CommandRequest{}, err
	}
	var params any
	if len(in.Params) > 0 && string(in.Params) != "null" {
		if err := json.Unmarshal(in.Params, &params); err != nil {
			return sdk.CommandRequest{}, err
		}
	}
	command := map[string]any{}
	if in.Method != "" {
		command[in.Method] = params
	}
	if writeMap, ok := params.(map[string]any); ok && in.Method == "write" {
		command["write"] = writeMap
	}
	return sdk.CommandRequest{DeviceID: deviceID, Command: command}, nil
}

func (b *mqttCommandBridge) publishResponse(client mqtt.Client, deviceID string, messageID string, err error) {
	values := map[string]any{"result": 0, "message": "success", "ts": time.Now().Unix()}
	if err != nil {
		values["result"] = 1
		values["errcode"] = "000"
		values["message"] = err.Error()
	}
	payload, _ := json.Marshal(map[string]any{"device_id": deviceID, "values": values})
	token := client.Publish("devices/command/response/"+messageID, 1, false, payload)
	token.Wait()
}

func deviceNumberFromTopic(topic string) string {
	parts := strings.Split(strings.Trim(topic, "/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}

func messageIDFromTopic(topic string) string {
	parts := strings.Split(strings.Trim(topic, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

//go:build plc4x

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	plc4go "github.com/apache/plc4x/plc4go/pkg/api"
	"github.com/apache/plc4x/plc4go/pkg/api/drivers"
	"github.com/apache/plc4x/plc4go/pkg/api/model"
	"github.com/apache/plc4x/plc4go/pkg/api/values"
)

type plcClient struct {
	driverManager plc4go.PlcDriverManager
}

func newPLCClient() *plcClient {
	manager := plc4go.NewPlcDriverManager()
	drivers.RegisterModbusTcpDriver(manager)
	drivers.RegisterModbusRtuDriver(manager)
	drivers.RegisterModbusAsciiDriver(manager)
	drivers.RegisterS7Driver(manager)
	return &plcClient{driverManager: manager}
}

func (c *plcClient) ReadTags(ctx context.Context, cfg plcConfig) (map[string]any, error) {
	opCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	conn, err := c.driverManager.GetConnection(opCtx, cfg.ConnectionString)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if !conn.GetMetadata().CanRead() {
		return nil, fmt.Errorf("connection does not support read operations")
	}
	builder := conn.ReadRequestBuilder()
	for _, tag := range cfg.Tags {
		builder.AddTagAddress(tag.Name, tag.Address)
	}
	request, err := builder.Build()
	if err != nil {
		return nil, err
	}
	result := <-request.Execute(opCtx)
	if err := result.GetErr(); err != nil {
		return nil, err
	}
	response := result.GetResponse()
	values := make(map[string]any, len(cfg.Tags))
	for _, tag := range cfg.Tags {
		code := response.GetResponseCode(tag.Name)
		if code != model.PlcResponseCode_OK {
			values[tag.Name+"_error"] = code.GetName()
			continue
		}
		values[tag.Name] = plcValueToAny(response.GetValue(tag.Name))
	}
	return values, nil
}

func (c *plcClient) WriteTag(ctx context.Context, cfg plcConfig, tag plcTag, value any) error {
	opCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.ConnectionString)), "simulated://") {
		return fmt.Errorf("simulated:// driver in PLC4X does not implement write value conversion yet; please use read-only tags for simulator testing")
	}

	conn, err := c.driverManager.GetConnection(opCtx, cfg.ConnectionString)
	if err != nil {
		return err
	}
	defer conn.Close()

	if !conn.GetMetadata().CanWrite() {
		return fmt.Errorf("connection does not support write operations")
	}
	request, err := conn.WriteRequestBuilder().
		AddTagAddress(tag.Name, tag.Address, value).
		Build()
	if err != nil {
		return err
	}
	result := <-request.Execute(opCtx)
	if err := result.GetErr(); err != nil {
		return err
	}
	code := result.GetResponse().GetResponseCode(tag.Name)
	if code != model.PlcResponseCode_OK {
		return fmt.Errorf("plc write response for %s: %s", tag.Name, code.GetName())
	}
	return nil
}

func plcValueToAny(v values.PlcValue) any {
	if v == nil || v.IsNull() {
		return nil
	}
	switch v.GetPlcValueType() {
	case values.BOOL:
		return v.GetBool()
	case values.SINT:
		return v.GetInt8()
	case values.INT:
		return v.GetInt16()
	case values.DINT:
		return v.GetInt32()
	case values.LINT:
		return v.GetInt64()
	case values.USINT:
		return v.GetUint8()
	case values.UINT:
		return v.GetUint16()
	case values.UDINT:
		return v.GetUint32()
	case values.ULINT:
		return v.GetUint64()
	case values.REAL:
		return v.GetFloat32()
	case values.LREAL:
		return v.GetFloat64()
	case values.STRING, values.WSTRING:
		return v.GetString()
	case values.TIME:
		return v.GetTime().Format(time.RFC3339Nano)
	case values.DATE:
		return v.GetDate().Format("2006-01-02")
	case values.DATE_AND_TIME:
		return v.GetDateTime().Format(time.RFC3339Nano)
	case values.TIME_OF_DAY:
		return v.GetTime().Format(time.RFC3339Nano)
	}
	switch {
	case v.IsDuration():
		return v.GetDuration().String()
	case v.IsList():
		items := make([]any, 0, v.GetLength())
		for i := uint32(0); i < v.GetLength(); i++ {
			items = append(items, plcValueToAny(v.GetIndex(i)))
		}
		return items
	case v.IsStruct():
		out := map[string]any{}
		for _, key := range v.GetKeys() {
			out[key] = plcValueToAny(v.GetValue(key))
		}
		return out
	default:
		return v.String()
	}
}

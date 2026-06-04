//go:build plc4x

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	spivalues "github.com/apache/plc4x/plc4go/spi/values"
)

func TestPlcValueToAnyPreservesNumericTypes(t *testing.T) {
	if got := plcValueToAny(spivalues.NewPlcREAL(12.5)); got != float32(12.5) {
		t.Fatalf("expected float32 12.5, got %#v", got)
	}
	if got := plcValueToAny(spivalues.NewPlcINT(42)); got != int16(42) {
		t.Fatalf("expected int16 42, got %#v", got)
	}
}

func TestWriteTagRejectsSimulatedWrites(t *testing.T) {
	client := newPLCClient()
	err := client.WriteTag(context.Background(), plcConfig{
		ConnectionString: "simulated://127.0.0.1",
		Timeout:          time.Second,
	}, plcTag{
		Name:     "writer",
		Address:  "STATE/test:BOOL",
		Writable: true,
	}, true)
	if err == nil || !strings.Contains(err.Error(), "does not implement write value conversion yet") {
		t.Fatalf("expected simulated write guard error, got %v", err)
	}
}

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	sdk "github.com/thingspanel/device-connector-sdk-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	info := sdk.FromEnv()
	if info.ServiceIdentifier == "" {
		slog.Error("CONNECTOR_SERVICE_IDENTIFIER is required")
		os.Exit(1)
	}
	if info.InstanceID == "" {
		slog.Error("CONNECTOR_INSTANCE_ID is required")
		os.Exit(1)
	}

	handler := newHandler()
	startMQTTCommandBridge(ctx, info, handler)
	startPLCTelemetry(ctx, handler)

	server := sdk.NewServer(info, handler)
	if err := server.Run(ctx); err != nil {
		slog.Error("connector exited", "error", err)
		os.Exit(1)
	}
}

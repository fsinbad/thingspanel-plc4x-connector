//go:build !plc4x

package main

import (
	"context"
	"fmt"
)

type plcClient struct{}

func newPLCClient() *plcClient {
	return &plcClient{}
}

func (c *plcClient) ReadTags(context.Context, plcConfig) (map[string]any, error) {
	return nil, fmt.Errorf("real PLC4X client is disabled; run or build with -tags plc4x")
}

func (c *plcClient) WriteTag(context.Context, plcConfig, plcTag, any) error {
	return fmt.Errorf("real PLC4X client is disabled; run or build with -tags plc4x")
}

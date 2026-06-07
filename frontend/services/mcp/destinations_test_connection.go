package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/odigos-io/odigos/common"
	testconnection "github.com/odigos-io/odigos/frontend/services/test_connection"
)

// destinations_test_connection.go exposes destination connectivity validation
// without persisting a Destination CR — the "did I get the API key right"
// check the agent should call before create_destination.

// destinationStub implements config.ExporterConfigurer for the test runner
// without requiring a persisted Destination CR.
type destinationStub struct {
	destType common.DestinationType
	signals  []common.ObservabilitySignal
	config   map[string]string
}

func (d *destinationStub) GetSignals() []common.ObservabilitySignal { return d.signals }
func (d *destinationStub) GetType() common.DestinationType          { return d.destType }
func (d *destinationStub) GetID() string                            { return "mcp-test-connection" }
func (d *destinationStub) GetConfig() map[string]string             { return d.config }

func testDestinationConnectionHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		destType, err := req.RequireString("type")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fields, err := requireStringMap(req, "fields")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		signalsRaw := optStringSlice(req, "signals")
		if len(signalsRaw) == 0 {
			signalsRaw = []string{string(common.TracesObservabilitySignal)}
		}
		signals := make([]common.ObservabilitySignal, 0, len(signalsRaw))
		for _, s := range signalsRaw {
			signals = append(signals, common.ObservabilitySignal(s))
		}
		stub := &destinationStub{
			destType: common.DestinationType(destType),
			signals:  signals,
			config:   fields,
		}
		result := testconnection.TestConnection(ctx, stub)
		return withGuidance(result, "If Succeeded=false, fix the fields (often a wrong API key or endpoint) before calling create_destination.", []NextStep{
			{When: "if the test passed, to create the destination", Tool: "create_destination", Args: map[string]any{"type": destType}},
		})
	}
}

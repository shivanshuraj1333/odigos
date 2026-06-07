// Command odigos-ui is the OSS Odigos frontend: a Go binary that serves the
// embedded Next.js webapp + GraphQL + remote-CLI HTTP endpoints. All setup
// logic lives in the importable `frontend/server` package so out-of-tree
// builds (notably odigos-enterprise's UI image) can reuse it and attach
// additional mounts (e.g. the enterprise MCP server) via RouterOpts.ExtraMounts.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	ctrl "sigs.k8s.io/controller-runtime"

	"k8s.io/klog/v2"

	"github.com/odigos-io/odigos/api/k8sconsts"
	"github.com/odigos-io/odigos/common"
	commonlogger "github.com/odigos-io/odigos/common/logger"
	"github.com/odigos-io/odigos/frontend/server"
	mcpserver "github.com/odigos-io/odigos/frontend/services/mcp"
	"github.com/odigos-io/odigos/frontend/version"
)

func main() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	flags := server.ParseFlags()

	if flags.Version {
		fmt.Printf("version.Info{Version:'%s', GitCommit:'%s', BuildDate:'%s'}\n", version.OdigosVersion, version.OdigosCommit, version.OdigosDate)
		return
	}

	commonlogger.Init(os.Getenv("ODIGOS_LOG_LEVEL"), "ui")
	logger := commonlogger.ToLogr()
	ctrl.SetLogger(logger)
	klog.SetLogger(logger)
	log := commonlogger.LoggerCompat().With("subsystem", "startup")

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(sigCh)
		cancel()
	}()

	go common.StartPprofServer(ctx, logger, int(k8sconsts.DefaultPprofEndpointPort))

	deps, err := server.Bootstrap(ctx, flags, logger)
	if err != nil {
		log.Error("bootstrap failed", "err", err)
		os.Exit(1)
	}

	wg, err := server.StartBackground(ctx, deps)
	if err != nil {
		log.Error("starting background goroutines failed", "err", err)
		os.Exit(1)
	}

	// The OSS frontend mounts its own MCP server (the full tool catalog) through
	// the very same RouterOpts.ExtraMounts seam the enterprise wrapper uses — so
	// k8s and enterprise share one mount mechanism. It is gated by
	// ODIGOS_MCP_ENABLED and returns nil (no mount) otherwise.
	r, err := server.BuildRouter(ctx, deps, server.RouterOpts{
		ExtraMounts: []func(*gin.Engine, *server.Deps){mountMCP},
	})
	if err != nil {
		log.Error("building router failed", "err", err)
		os.Exit(1)
	}

	if err := server.ServeAndWait(cancel, deps, r, sigCh, wg); err != nil {
		os.Exit(1)
	}
}

// mountMCP attaches the MCP server at /mcp when ODIGOS_MCP_ENABLED is set. The
// server.Deps already carry exactly what the MCP tools need (k8s cache client,
// metrics consumer, Prometheus API, profile store), so the agent surface reuses
// the UI's own services rather than re-implementing data access.
func mountMCP(r *gin.Engine, deps *server.Deps) {
	if h := mcpserver.Handler(mcpserver.Deps{
		Logger:          deps.Logger,
		MetricsConsumer: deps.OdigosMetrics,
		PromAPI:         deps.PromAPI,
		K8sCacheClient:  deps.K8sCacheClient,
		ProfileStore:    deps.ProfileStore,
	}); h != nil {
		r.Any("/mcp", gin.WrapH(h))
	}
}

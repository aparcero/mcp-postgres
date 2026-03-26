package main

import (
	"context"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/aparcero/mcp-postgres/internal/config"
	"github.com/aparcero/mcp-postgres/internal/logging"
	mcpserver "github.com/aparcero/mcp-postgres/internal/mcp"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	logger, err := logging.NewLogger(cfg.LogLevel, cfg.LogJSON)
	if err != nil {
		log.Fatalf("create logger: %v", err)
	}

	app, err := mcpserver.New(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to initialize server", "error", err)
		log.Fatalf("initialize server: %v", err)
	}
	defer app.Close()

	logger.Info("starting mcp-postgres server",
		"bootstrap_database", cfg.BootstrapDatabase,
		"mode", cfg.Mode,
		"idle_pool_ttl_seconds", int64(cfg.IdlePoolTTL.Seconds()),
		"idle_pool_cleanup_interval_seconds", int64(cfg.IdlePoolCleanup.Seconds()),
	)

	if err := app.Server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		logger.Error("server stopped with error", "error", err)
		log.Fatalf("run server: %v", err)
	}
}

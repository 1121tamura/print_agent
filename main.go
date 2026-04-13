package main

import (
	"context"
	"log/slog"
	"os"
	"sync"

	"print-agent/internal/config"
	"print-agent/internal/infrastructure/backend"
	"print-agent/internal/infrastructure/localapi"
	"print-agent/internal/infrastructure/printer"
	"print-agent/internal/infrastructure/redis"
	"print-agent/internal/infrastructure/servicehost"
	"print-agent/internal/worker"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	backendClient := backend.NewClient(cfg, logger)

	// agent.id が未設定の場合は Backend に自動登録
	if cfg.Agent.ID == "" {
		logger.Info("agent.id is empty, registering to backend")
		id, err := backendClient.Register()
		if err != nil {
			logger.Error("agent registration failed", "err", err)
			os.Exit(1)
		}
		if err := cfg.WriteAgentID(id); err != nil {
			logger.Error("failed to save agent_id to config", "err", err)
			os.Exit(1)
		}
		logger.Info("agent registered", "agent_id", id)
	}

	// 起動通知
	if err := backendClient.ReportStatus(backend.StatusOnline, "", ""); err != nil {
		logger.Warn("failed to report online status", "err", err)
	}

	prt := printer.NewWindowsPrinter(cfg, logger)
	consumer := redis.NewConsumer(cfg, logger)
	proc := worker.NewProcessor(cfg, logger, consumer, backendClient, prt)
	server := localapi.NewServer(cfg, logger, prt)

	// サービスモード／コンソールモードを自動判別して起動
	servicehost.Run(logger, func(ctx context.Context) error {
		var wg sync.WaitGroup
		wg.Add(2)
		go proc.Start(ctx, &wg)
		go server.Start(ctx, &wg)
		wg.Wait()

		consumer.Close() // XREADGROUP の無限待機を即座に解除
		return nil
	})
}

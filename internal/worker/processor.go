package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"print-agent/internal/config"
	"print-agent/internal/infrastructure/backend"
	"print-agent/internal/infrastructure/printer"
	"print-agent/internal/infrastructure/redis"
)

type Processor struct {
	cfg      *config.Config
	logger   *slog.Logger
	consumer *redis.Consumer
	backend  *backend.Client
	printer  printer.Printer
}

func NewProcessor(cfg *config.Config, logger *slog.Logger, consumer *redis.Consumer, backend *backend.Client, prt printer.Printer) *Processor {
	return &Processor{
		cfg:      cfg,
		logger:   logger,
		consumer: consumer,
		backend:  backend,
		printer:  prt,
	}
}

func (p *Processor) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	if p.cfg.Print.CleanupOnStartup {
		p.cleanupTempDir()
	}

	var iwg sync.WaitGroup
	iwg.Add(2)
	go p.runHeartbeat(ctx, &iwg)
	go p.runJobLoop(ctx, &iwg)
	iwg.Wait()
}

func (p *Processor) runHeartbeat(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	interval := time.Duration(p.cfg.Heartbeat.IntervalSec) * time.Second
	if interval == 0 {
		interval = 10 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.backend.SendHeartbeat(); err != nil {
				p.logger.Warn("heartbeat failed", "err", err)
			} else {
				p.logger.Debug("heartbeat sent")
			}
		}
	}
}

func (p *Processor) runJobLoop(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		if ctx.Err() != nil {
			return
		}

		jobID, msgID, err := p.consumer.Read(ctx)
		if err != nil {
			p.logger.Error("redis read failed", "err", err)
			continue
		}
		if jobID == "" {
			// メッセージなし（タイムアウト）、次のループへ
			continue
		}

		p.logger.Info("job received", "job_id", jobID)
		p.processJob(ctx, jobID, msgID)
	}
}

func (p *Processor) processJob(ctx context.Context, jobID, msgID string) {
	// ステップ2〜4: ジョブ詳細取得・PDF取得・temp保存
	tempPath, err := p.fetchAndSavePDF(jobID)
	if err != nil {
		p.logger.Error("fetch pdf failed", "job_id", jobID, "err", err)
		p.reportAndAck(ctx, jobID, msgID, false, err.Error())
		return
	}

	// ステップ5: 印刷
	printErr := p.printer.Print(tempPath)
	if printErr != nil {
		p.logger.Error("print failed", "job_id", jobID, "err", printErr)
	}

	// ステップ6: 結果返却（成功・失敗どちらでも）
	// ステップ7: temp削除
	// ステップ8: ACK
	p.reportAndAck(ctx, jobID, msgID, printErr == nil, errString(printErr))

	if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
		p.logger.Warn("temp file remove failed", "path", tempPath, "err", err)
	}
}

func (p *Processor) fetchAndSavePDF(jobID string) (string, error) {
	// ジョブ詳細取得
	_, err := p.backend.GetJob(jobID)
	if err != nil {
		return "", fmt.Errorf("get job: %w", err)
	}

	// PDF取得
	pdf, err := p.backend.GetPDF(jobID)
	if err != nil {
		return "", fmt.Errorf("get pdf: %w", err)
	}

	// temp保存
	tempPath := filepath.Join(p.cfg.Storage.TempDir, jobID+".pdf")
	if err := os.WriteFile(tempPath, pdf, 0600); err != nil {
		return "", fmt.Errorf("save pdf: %w", err)
	}

	p.logger.Info("pdf saved", "job_id", jobID, "path", tempPath)
	return tempPath, nil
}

func (p *Processor) reportAndAck(ctx context.Context, jobID, msgID string, success bool, errMsg string) {
	if err := p.backend.ReportResult(jobID, success, errMsg); err != nil {
		p.logger.Error("report result failed", "job_id", jobID, "err", err)
	}

	if err := p.consumer.Ack(ctx, msgID); err != nil {
		p.logger.Error("ack failed", "job_id", jobID, "err", err)
	}
}

func (p *Processor) cleanupTempDir() {
	entries, err := os.ReadDir(p.cfg.Storage.TempDir)
	if err != nil {
		if !os.IsNotExist(err) {
			p.logger.Warn("cleanup temp dir failed", "err", err)
		}
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(p.cfg.Storage.TempDir, entry.Name())
		if err := os.Remove(path); err != nil {
			p.logger.Warn("cleanup file failed", "path", path, "err", err)
		} else {
			p.logger.Info("cleanup temp file", "path", path)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

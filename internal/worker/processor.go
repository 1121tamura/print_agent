package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

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

	p.runJobLoop(ctx)
}

func (p *Processor) runJobLoop(ctx context.Context) {
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
	// ジョブ詳細取得・PDF取得・temp保存
	tempPath, err := p.fetchAndSavePDF(jobID)
	if err != nil {
		p.logger.Error("fetch pdf failed", "job_id", jobID, "err", err)
		p.reportStatus(backend.StatusError, jobID, err.Error())
		p.ack(ctx, jobID, msgID)
		return
	}

	// 印刷開始通知（Backend への到達確認・タイムアウト判定のために必須）
	p.reportStatus(backend.StatusPrinting, jobID, "")

	// 印刷実行
	printErr := p.printer.Print(tempPath)
	if printErr != nil {
		p.logger.Error("print failed", "job_id", jobID, "err", printErr)
		p.reportStatus(backend.StatusError, jobID, printErr.Error())
	} else {
		p.logger.Info("print success", "job_id", jobID)
		p.reportStatus(backend.StatusSuccess, jobID, "")
	}

	// temp削除
	if p.cfg.Print.DeleteTempAfterPrint {
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
			p.logger.Warn("temp file remove failed", "path", tempPath, "err", err)
		}
	}

	// ACK
	p.ack(ctx, jobID, msgID)
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

func (p *Processor) reportStatus(status, jobID, errMsg string) {
	if err := p.backend.ReportStatus(status, jobID, errMsg); err != nil {
		p.logger.Error("report status failed", "status", status, "job_id", jobID, "err", err)
	}
}

func (p *Processor) ack(ctx context.Context, jobID, msgID string) {
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

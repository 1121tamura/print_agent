//go:build windows

package printer

import (
	"fmt"
	"log/slog"

	"print-agent/internal/config"
)

type WindowsPrinter struct {
	cfg    *config.Config
	logger *slog.Logger
}

func NewWindowsPrinter(cfg *config.Config, logger *slog.Logger) Printer {
	return &WindowsPrinter{cfg: cfg, logger: logger}
}

func (p *WindowsPrinter) GetDefaultPrinter() (string, error) {
	// TODO: Windows API (GetDefaultPrinter) で実装
	return "", fmt.Errorf("not implemented")
}

func (p *WindowsPrinter) Print(filePath string) error {
	// TODO: SumatraPDF などで印刷実装
	return fmt.Errorf("not implemented")
}

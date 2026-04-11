//go:build !windows

package printer

import (
	"log/slog"

	"print-agent/internal/config"
)

type StubPrinter struct{}

func NewWindowsPrinter(cfg *config.Config, logger *slog.Logger) Printer {
	return &StubPrinter{}
}

func (p *StubPrinter) GetDefaultPrinter() (string, error) {
	return "stub-printer", nil
}

func (p *StubPrinter) Print(filePath string) error {
	return nil
}

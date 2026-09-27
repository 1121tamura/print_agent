//go:build !windows

package servicehost

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// Run はコンソールモードで実行する（開発時やwindows。)
// SIGINT / SIGTERM を受け取るまで fn を実行し続ける。
func Run(logger *slog.Logger, fn func(ctx context.Context) error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		errCh <- fn(ctx)
	}()

	select {
	case <-quit:
		logger.Info("shutting down")
		cancel()
	case err := <-errCh:
		if err != nil {
			logger.Error("service error", "err", err)
		}
	}
}

//go:build windows

package servicehost

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows/svc"
)

const serviceName = "PrintAgent"

// Run はWindows サービスとして動作しているか判定し、
// サービスモードならサービスマネージャーと通信しながら fn を実行する。
// コンソールから起動された場合はシグナル待ちで fn を実行する。
func Run(logger *slog.Logger, fn func(ctx context.Context) error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		logger.Error("failed to detect service mode", "err", err)
		runConsole(logger, fn)
		return
	}

	if isService {
		runService(logger, fn)
	} else {
		runConsole(logger, fn)
	}
}

// runService は Windows サービスマネージャーと通信しながら fn を実行する。
func runService(logger *slog.Logger, fn func(ctx context.Context) error) {
	err := svc.Run(serviceName, &handler{logger: logger, fn: fn})
	if err != nil {
		logger.Error("service run failed", "err", err)
	}
}

// runConsole はコンソールから起動された場合のシグナル待ち。
func runConsole(logger *slog.Logger, fn func(ctx context.Context) error) {
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

// handler は Windows サービスマネージャーからの制御コマンドを処理する。
type handler struct {
	logger *slog.Logger
	fn     func(ctx context.Context) error
}

// Execute はサービスマネージャーから呼ばれるメインループ。
// サービスマネージャーとは
// タスクマネージャーの「サービス」タブや services.msc で見えるやつ
//
// // Start コマンドで fn を起動し、Stop / Shutdown で ctx をキャンセルして終了する。

func (h *handler) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	// サービスマネージャーに「起動中」を通知
	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- h.fn(ctx)
	}()

	// サービスマネージャーに「起動完了」を通知
	// AcceptStop: sc.exe stop を受け付ける
	// AcceptShutdown: Windows シャットダウン時の通知を受け付ける
	status <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	// ＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊
	// 「サービスが動いている間、2つの終了条件を監視する」処理
	// 終了条件1: fn の終了
	// 	proc/server がクラッシュ
	//   ↓
	// errCh にエラーが送られる
	//   ↓
	// サービスマネージャーに通知して終了
	// ＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊
	// 終了条件2: サービスマネージャーからの Stop / Shutdown コマンド
	// 	sc.exe stop または Windows シャットダウン
	//   ↓
	// サービスマネージャーから req にコマンドが送られる
	//   ↓
	// cancel() で proc/server に停止を伝える
	//   ↓
	// errCh で終了を待ってからサービス終了
	// ＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊
	for {
		select {
		case err := <-errCh:
			if err != nil {
				h.logger.Error("service error", "err", err)
			}
			status <- svc.Status{State: svc.StopPending}
			return false, 0

		case cmd := <-req:
			switch cmd.Cmd {
			case svc.Stop, svc.Shutdown:
				// sc.exe stop または Windows シャットダウン
				h.logger.Info("shutting down", "cmd", cmd.Cmd)
				status <- svc.Status{State: svc.StopPending}
				cancel()
				<-errCh // fn の終了を待つ→印刷中にサービス停止されても、印刷処理を完走してから終了するため、この順番で待つ
				return false, 0
			}
		}
	}
}

//go:build windows

package printer

import (
	"fmt"
	"log/slog"
	"syscall"
	"time"
	"unsafe"

	"print-agent/internal/config"
)

var (
	winspool              = syscall.NewLazyDLL("winspool.drv")
	procGetDefaultPrinter = winspool.NewProc("GetDefaultPrinterW")
	// ShellExecuteW 関数を Go から呼び出す
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

type WindowsPrinter struct {
	cfg    *config.Config
	logger *slog.Logger
}

func NewWindowsPrinter(cfg *config.Config, logger *slog.Logger) Printer {
	return &WindowsPrinter{cfg: cfg, logger: logger}
}

// GetDefaultPrinter は Windows の「通常使うプリンター」名を返す。
func (p *WindowsPrinter) GetDefaultPrinter() (string, error) {
	// 1回目: バッファサイズを取得
	var size uint32
	procGetDefaultPrinter.Call(0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return "", fmt.Errorf("no default printer configured")
	}

	// 2回目: 実際の名前を取得
	buf := make([]uint16, size)
	ret, _, err := procGetDefaultPrinter.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ret == 0 {
		return "", fmt.Errorf("GetDefaultPrinterW failed: %w", err)
	}

	return syscall.UTF16ToString(buf), nil
}

// Print は PDF ファイルを通常使うプリンターへ印刷する。
// GetDefaultPrinter は Windows の「通常使うプリンター」名を返す。
//※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊
// ShellExecute とは
// Windows の「ファイルを右クリック → 印刷」と同じ操作をプログラムから行う API です。
// エクスプローラーで PDF を右クリック
//   ├── 開く      → verb: "open"
//   ├── 印刷      → verb: "print"  ← これ今回しよする
//   └── 編集      → verb: "edit"
//※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※※＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊＊
// verb: "print" の動作
// ShellExecute("print", "C:\...\job-xxx.pdf")
//   ↓
// Windows が PDF に関連付けられたアプリを探す
//   ↓
// 例: Microsoft Edge / Adobe Reader / Foxit など
//   ↓
// そのアプリが「通常使うプリンター」に印刷を実行
//   ↓
// 印刷スプーラに投入して ShellExecute は返る（非同期）

func (p *WindowsPrinter) Print(filePath string) error {
	printerName, err := p.GetDefaultPrinter()
	if err != nil {
		return fmt.Errorf("get default printer: %w", err)
	}
	p.logger.Info("printing", "printer", printerName, "file", filePath)

	verb, _ := syscall.UTF16PtrFromString("print")
	file, _ := syscall.UTF16PtrFromString(filePath)

	// ShellExecute で "print" verb を実行
	// 戻り値が 32 より大きければ成功
	// 歴史的な理由で、32以下の値はエラーコードとして予約されてるため
	// 値	意味
	// 2	ファイルが見つからない
	// 3	パスが見つからない
	// 5	アクセス拒否
	// 31	関連付けられたアプリがない
	// 33以上	成功（ウィンドウハンドル）

	// ShellExecute実行
	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		0,
		1, // SW_SHOWNORMAL
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecute print failed: error code %d", ret)
	}

	// ShellExecute は非同期のため、印刷スプーラへの投入を待つ
	// 	printer.Print(tempPath)
	//   └─ ShellExecute 呼び出し
	//   └─ time.Sleep(3秒)  ← スプーラ投入を待つ
	//   └─ return
	// ↓
	// os.Remove(tempPath)  ← ここで削除
	// Sleep(3秒) が終わってから Print() が返るので、スプーラ投入後に削除されます。

	time.Sleep(3 * time.Second)

	p.logger.Info("print job submitted", "printer", printerName)
	return nil
}

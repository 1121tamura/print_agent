package printer

// Printer はプリンター操作のインターフェース。
type Printer interface {
	GetDefaultPrinter() (string, error)
	Print(filePath string) error
}

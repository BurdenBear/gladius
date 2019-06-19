package gladius

import (
	"os"

	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("fdtrader")

func init() {
	InitLog(logging.DEBUG, "")
}

func GetLogger() *logging.Logger {
	return log
}

func InitLog(level logging.Level, filename string) {
	var format = logging.MustStringFormatter(
		`%{time:2001/02/03 15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{message}`,
	)
	backends := []logging.Backend{}
	backend := logging.NewLogBackend(os.Stderr, "", 0)
	backendFormatter := logging.NewBackendFormatter(backend, format)
	// Only errors and more severe messages should be sent to backend1
	backendLeveled := logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(level, "fdtrader")
	backends = append(backends, backendLeveled)
	if filename != "" {
		file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
		if err == nil {
			backend := logging.NewLogBackend(file, "", 0)
			backendFormatter := logging.NewBackendFormatter(backend, format)
			backendLeveled := logging.AddModuleLevel(backendFormatter)
			backendLeveled.SetLevel(level, "fdtrader")
			backends = append(backends, backendLeveled)
		}
	}
	logging.SetBackend(backends...)
}

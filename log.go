package gladius

import (
	"errors"
	"os"

	"github.com/op/go-logging"
)

var _loggerName = "gladius"

var logger = logging.MustGetLogger(_loggerName)

func NewLogConfig() *LogConfig {
	c := &LogConfig{
		Console: ConsoleLogConfig{
			Enabled: true,
			Level:   "DEBUG",
		},
		File: FileLogConfig{
			Enabled: false,
		},
	}
	return c
}

func init() {
	InitLog(NewLogConfig())
}

func GetLogger() *logging.Logger {
	return logger
}

type ConsoleLogConfig struct {
	Enabled bool   `json:"enabled"`
	Level   string `json:"level"`
}

type FileLogConfig struct {
	Enabled bool   `json:"enabled"`
	Level   string `json:"level"`
	Path    string `json:"path"`
}

type LogConfig struct {
	Console ConsoleLogConfig
	File    FileLogConfig
}

func InitLog(c *LogConfig) {
	var format = logging.MustStringFormatter(
		`%{time:2006/01/02 15:04:05.000} %{shortfile} %{shortfunc} [%{level:.4s}] %{message}`,
	)
	backends := []logging.Backend{}
	if c.Console.Enabled {
		backend := logging.NewLogBackend(os.Stderr, "", 0)
		backendFormatter := logging.NewBackendFormatter(backend, format)
		// Only errors and more severe messages should be sent to backend1
		backendLeveled := logging.AddModuleLevel(backendFormatter)
		level, err := logging.LogLevel(c.Console.Level)
		if err != nil {
			panic(err)
		}
		backendLeveled.SetLevel(level, _loggerName)
		backends = append(backends, backendLeveled)
	}
	if c.File.Enabled {
		if c.File.Path == "" {
			panic(errors.New("must set the path of log file if turn file logging on, please check the config"))
		}
		file, err := os.OpenFile(c.File.Path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
		if err == nil {
			backend := logging.NewLogBackend(file, "", 0)
			backendFormatter := logging.NewBackendFormatter(backend, format)
			backendLeveled := logging.AddModuleLevel(backendFormatter)
			level, err := logging.LogLevel(c.File.Level)
			if err != nil {
				panic(err)
			}
			backendLeveled.SetLevel(level, _loggerName)
			backends = append(backends, backendLeveled)
		} else {
			panic(err)
		}
	}
	logging.SetBackend(backends...)
}

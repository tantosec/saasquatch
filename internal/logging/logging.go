package logging

import (
	"os"
	"time"

	"github.com/charmbracelet/log"
)

var (
	globalLogger *log.Logger
)

func InitLogger(verbose int) {
	logLevel := log.InfoLevel
	if verbose > 0 {
		logLevel = log.DebugLevel
	}

	globalLogger = log.NewWithOptions(os.Stderr, log.Options{
		Level:      logLevel,
		TimeFormat: time.Kitchen,
		Prefix:     "",
	})

}

func GetLogger() *log.Logger {
	if globalLogger == nil {
		globalLogger = log.New(os.Stderr)
	}
	return globalLogger
}

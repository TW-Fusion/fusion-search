package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.SugaredLogger

func InitLogger(format, level string) error {
	var config zap.Config

	levelEnum := zap.InfoLevel
	switch level {
	case "DEBUG":
		levelEnum = zap.DebugLevel
	case "INFO":
		levelEnum = zap.InfoLevel
	case "WARN":
		levelEnum = zap.WarnLevel
	case "ERROR":
		levelEnum = zap.ErrorLevel
	}

	if format == "json" {
		config = zap.Config{
			Level:       zap.NewAtomicLevelAt(levelEnum),
			Development: false,
			Encoding:    "json",
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:        "timestamp",
				LevelKey:       "level",
				NameKey:        "logger",
				CallerKey:      "caller",
				MessageKey:     "msg",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.LowercaseLevelEncoder,
				EncodeTime:     zapcore.ISO8601TimeEncoder,
				EncodeDuration: zapcore.SecondsDurationEncoder,
				EncodeCaller:   zapcore.ShortCallerEncoder,
			},
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
	} else {
		config = zap.Config{
			Level:            zap.NewAtomicLevelAt(levelEnum),
			Development:      false,
			Encoding:         "console",
			EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
	}

	logger, err := config.Build()
	if err != nil {
		return err
	}

	Logger = logger.Sugar()
	zap.ReplaceGlobals(logger)

	return nil
}

func GetLogger() *zap.SugaredLogger {
	if Logger == nil {
		// Initialize with defaults if not already initialized
		if err := InitLogger("json", "INFO"); err != nil {
			panic(err)
		}
	}
	return Logger
}

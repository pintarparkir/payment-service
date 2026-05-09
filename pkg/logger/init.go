package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

// NewLogger initialises the package-level zap logger.
//   - local env: console encoder, stdout only (no lumberjack — /var/log writes
//     fail without root, and dev doesn't need rotated files anyway).
//   - non-local: JSON encoder + stdout + rotated file under /var/log/<app>/app.log.
func NewLogger(appName, appEnv string) error {
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var encoder zapcore.Encoder
	if appEnv == "local" {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	var writer zapcore.WriteSyncer
	if appEnv == "local" {
		writer = zapcore.AddSync(consoleStdout())
	} else {
		rotator := &lumberjack.Logger{
			Filename:   "/var/log/" + appName + "/app.log",
			MaxSize:    100,
			MaxBackups: 7,
			MaxAge:     30,
			Compress:   true,
		}
		writer = zapcore.NewMultiWriteSyncer(zapcore.AddSync(rotator), zapcore.AddSync(consoleStdout()))
	}

	core := zapcore.NewCore(encoder, writer, zapcore.InfoLevel)
	instance = zap.New(core, zap.AddCaller(), zap.Fields(zap.String("app", appName), zap.String("env", appEnv)))
	return nil
}

package logger

// Package-level facades — keep call-sites short: logger.Info(ctx, "msg", fields).

// Info writes an info-level log entry with optional structured fields.
func Info(ctx interface{}, msg string, fields map[string]interface{}) {
	if instance == nil {
		return
	}
	instance.Info(msg, fieldsFromMap(ctx, fields)...)
}

// Warn writes a warn-level log entry.
func Warn(ctx interface{}, msg string, fields map[string]interface{}) {
	if instance == nil {
		return
	}
	instance.Warn(msg, fieldsFromMap(ctx, fields)...)
}

// Error writes an error-level log entry.
func Error(ctx interface{}, msg string, fields map[string]interface{}) {
	if instance == nil {
		return
	}
	instance.Error(msg, fieldsFromMap(ctx, fields)...)
}

// Fatal writes a fatal-level log entry and calls os.Exit(1).
func Fatal(ctx interface{}, msg string, fields map[string]interface{}) {
	if instance == nil {
		return
	}
	instance.Fatal(msg, fieldsFromMap(ctx, fields)...)
}

// Sync flushes pending log entries; call before exit.
func Sync() error {
	if instance == nil {
		return nil
	}
	return instance.Sync()
}

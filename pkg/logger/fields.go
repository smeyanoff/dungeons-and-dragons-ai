package logger

import (
	"time"

	"go.uber.org/zap"
)

// Helper функции для создания часто используемых полей

// String создает поле типа string
func String(key, value string) zap.Field {
	return zap.String(key, value)
}

// Int создает поле типа int
func Int(key string, value int) zap.Field {
	return zap.Int(key, value)
}

// Uint создает поле типа uint
func Uint(key string, value uint) zap.Field {
	return zap.Uint(key, value)
}

// Int64 создает поле типа int64
func Int64(key string, value int64) zap.Field {
	return zap.Int64(key, value)
}

// Uint64 создает поле типа uint64
func Uint64(key string, value uint64) zap.Field {
	return zap.Uint64(key, value)
}

// Float64 создает поле типа float64
func Float64(key string, value float64) zap.Field {
	return zap.Float64(key, value)
}

// Bool создает поле типа bool
func Bool(key string, value bool) zap.Field {
	return zap.Bool(key, value)
}

// Error создает поле типа error
func ErrorField(err error) zap.Field {
	return zap.Error(err)
}

// Duration создает поле типа time.Duration
func Duration(key string, value time.Duration) zap.Field {
	return zap.Duration(key, value)
}

// Any создает поле произвольного типа
func Any(key string, value interface{}) zap.Field {
	return zap.Any(key, value)
}

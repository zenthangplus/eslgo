package eslgo

import (
	"log"
)

type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

type NilLogger struct{}
type NormalLogger struct{}

func (l NormalLogger) Debug(format string, args ...interface{}) {
	log.Printf("DEBUG: "+format, args...)
}
func (l NormalLogger) Info(format string, args ...interface{}) {
	log.Printf("INFO: "+format, args...)
}
func (l NormalLogger) Warn(format string, args ...interface{}) {
	log.Printf("WARN: "+format, args...)
}
func (l NormalLogger) Error(format string, args ...interface{}) {
	log.Printf("ERROR: "+format, args...)
}

func (l NilLogger) Debug(string, ...interface{}) {}
func (l NilLogger) Info(string, ...interface{})  {}
func (l NilLogger) Warn(string, ...interface{})  {}
func (l NilLogger) Error(string, ...interface{}) {}

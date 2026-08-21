package logger

import (
	"time"

	"go.uber.org/zap/zapcore"
)

type LoggerConfig struct {
	Level         string                `json:"level" bson:"level"`
	Path          string                `json:"path" bson:"path"`
	Name          string                `json:"name" bson:"name"`
	MaxSize       int                   `json:"maxSize" bson:"maxSize"`
	Rotationtime  int                   `json:"rotationtime" bson:"rotationtime"`
	SecretValues  []string              `json:"secretKeys" bson:"secretKeys"`
	EncoderConfig zapcore.EncoderConfig `json:"encoderConfig" bson:"encoderConfig"`
	IsEnabled     bool                  `json:"isEnabled" bson:"isEnabled"`
}

type InfoData struct {
	Date time.Time   `json:"date"`
	Info interface{} `json:"data"`
}

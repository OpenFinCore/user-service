package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

var ZapLogger *zap.Logger

type OverloadedEncoder struct {
	zapcore.Encoder
	secretKey []string
}

var configuration LoggerConfig

func LoggerInit(conf LoggerConfig) *zap.Logger {
	encoder := zapcore.NewConsoleEncoder(conf.EncoderConfig)
	zapEncoder := OverloadedEncoder{encoder, conf.SecretValues}

	atomicLevel := zap.AtomicLevel{}
	err := atomicLevel.UnmarshalText([]byte(conf.Level))
	if err != nil {
		fmt.Println("Error occured while unmarshalling config log level into zap level", err.Error())
		panic(err.Error())
	}

	// Prepare writers
	var syncers []zapcore.WriteSyncer

	// Always log to Stdout (for PM2 and Docker capture)
	syncers = append(syncers, zapcore.AddSync(os.Stdout))

	// File rotation (only if path and name are provided in config)
	if conf.Path != "" && conf.Name != "" {
		fileRotationTime := time.Duration(conf.Rotationtime) * time.Minute
		fileRotation := zapcore.AddSync(NewTimeRotationWriter(conf.Name, conf.Path, fileRotationTime, conf.MaxSize))
		syncers = append(syncers, fileRotation)
	}

	// set up zap core configration
	core := zapcore.NewCore(
		zapEncoder,
		zapcore.NewMultiWriteSyncer(syncers...),
		atomicLevel,
	)

	ZapLogger = zap.New(core)

	configuration = conf

	return ZapLogger
}

// create a custom EncodeEntry method to filter out secret data
func (m OverloadedEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	filtered := make([]zapcore.Field, 0, len(fields))
	for _, field := range fields {
		res := field

		// to hide the sensitive data
		target := field.Interface
		reflectedType := reflect.TypeOf(target)
		if target != nil {
			if reflectedType.Kind() == reflect.Struct || reflectedType.Kind() == reflect.Ptr {
				targetMap := make(map[string]interface{})

				inrec, _ := json.Marshal(target)
				err := json.Unmarshal(inrec, &targetMap)
				if err == nil && len(targetMap) > 0 {
					for _, key := range m.secretKey {
						delete(targetMap, key)
					}

					res.Interface = targetMap
				}
			}
		}

		filtered = append(filtered, res)
	}

	return m.Encoder.EncodeEntry(entry, filtered)
}

package logger

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
)

func LogDebug(msg string, session string, val interface{}) {
	if configuration.IsEnabled {
		_, file, line, _ := runtime.Caller(1)
		logMsg := formatLogMsg(DEBUG, file, session, msg, line)
		var data string
		err, ok := val.(error)
		if ok {
			data = err.Error()
		} else {
			data = fmt.Sprintf("%v", val)
		}
		ZapLogger.Debug(logMsg, zap.Any("data", data))
	}
}

func LogDebug2(msg string, session string, val interface{}) {
	if configuration.IsEnabled {
		_, file, line, _ := runtime.Caller(1)
		logMsg := formatLogMsg(DEBUG2, file, session, msg, line)
		var data string
		err, ok := val.(error)
		if ok {
			data = err.Error()
		} else {
			data = fmt.Sprintf("%v", val)
		}
		ZapLogger.Debug(logMsg, zap.Any("data", data))
	}
}

func LogInfo(msg string, session string, val ...any) {
	if configuration.IsEnabled {
		_, file, line, _ := runtime.Caller(1)
		logMsg := formatLogMsg(INFO, file, session, msg, line)
		ZapLogger.Info(logMsg)
	}
}

func LogWarning(session string, val interface{}) {
	if configuration.IsEnabled {
		_, file, line, _ := runtime.Caller(1)
		logMsg := formatLogMsg(WARNING, file, session, "", line)
		var data string
		err, ok := val.(error)
		if ok {
			data = err.Error()
		} else {
			data = fmt.Sprintf("%v", val)
		}
		ZapLogger.Warn(logMsg, zap.Any("data", data))
	}
}

func LogError(session string, val interface{}) {
	if configuration.IsEnabled {
		_, file, line, _ := runtime.Caller(1)
		logMsg := formatLogMsg(ERROR, file, session, "", line)

		var data string
		err, ok := val.(error)
		if ok {
			data = err.Error()
		} else {
			data = fmt.Sprintf("%v", val)
		}
		ZapLogger.Error(logMsg, zap.Any("data", data))
		fmt.Println(logMsg)
	}
}

func LogFatal(session string, val interface{}) {
	if configuration.IsEnabled {
		_, file, line, _ := runtime.Caller(1)
		logMsg := formatLogMsg(FATAL, file, session, "", line)
		var data string
		err, ok := val.(error)
		if ok {
			data = err.Error()
		} else {
			data = fmt.Sprintf("%v", val)
		}
		ZapLogger.Fatal(logMsg, zap.Any("data", data))
	}
}

func LogPanic(session string, val interface{}) {
	if configuration.IsEnabled {
		_, file, line, _ := runtime.Caller(1)
		logMsg := formatLogMsg(PANIC, file, session, "", line)
		var data string
		err, ok := val.(error)
		if ok {
			data = err.Error()
		} else {
			data = fmt.Sprintf("%v", val)
		}
		ZapLogger.Panic(logMsg, zap.Any("data", data))
	}
}

func formatPath(path string) string {
	formattedPath := path
	splitedString := strings.Split(path, "/")
	pathLen := len(splitedString)

	if pathLen > 2 {
		formattedPath = fmt.Sprintf("%v", strings.Join(splitedString[pathLen-2:], "/"))
	}

	return formattedPath
}

func formatLogMsg(level, path, sessionId, msg string, line int) string {
	filePath := formatPath(path)
	curDateStr := time.Now().Format("2006-01-02 15:04:05.000")
	fileDetail := fmt.Sprintf("%s:%d", filePath, line)
	return fmt.Sprintf("%-8s    =>  %-35s  :::  %s   %s   %s", strings.ToUpper(level), fileDetail, curDateStr, sessionId, msg)
}

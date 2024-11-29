package main

import (
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)


type (
	CustomLoggerWriter struct{
		extraWeiter map[string] *websocket.Conn
	}
	CustomLogger struct{
		*logrus.Logger
		*CustomLoggerWriter
	}
)

func (lw *CustomLoggerWriter)Write(data []byte) (int, error){
	for _,w := range lw.extraWeiter{
		if w == nil{
			continue
		}
		w.WriteMessage(websocket.TextMessage, data)
	}
	return os.Stdout.Write(data)
}

func (lw *CustomLoggerWriter) AddWriter(key string,w *websocket.Conn){
	if lw.extraWeiter == nil{
		lw.extraWeiter = make(map[string] *websocket.Conn)
	}
	lw.extraWeiter[key] = w
}
func (lw *CustomLoggerWriter) RemoveWriter(key string){
	delete(lw.extraWeiter,key)
}
func (l *CustomLogger)AddWriter(key string,w *websocket.Conn){
	l.CustomLoggerWriter.AddWriter(key,w)
}
func (l *CustomLogger)RemoveWriter(key string){
	l.CustomLoggerWriter.RemoveWriter(key)
}

var(
	LOGGER_WRITE = &CustomLoggerWriter{}
	LOGGER =initLogger(LOGGER_WRITE)
)
func initLogger(lw *CustomLoggerWriter) *CustomLogger{
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
		ForceColors:     true,
	})
	logger.Out = lw
	return &CustomLogger{
		CustomLoggerWriter:lw,
		Logger:logger,
	}
}
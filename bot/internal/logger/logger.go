package logger

import (
	"io"
	"log"
	"os"
)

const (
	LevelError = iota
	LevelInfo
	LevelDebug
)

var (
	Error *log.Logger
	Info  *log.Logger
	Debug *log.Logger
)

func Setup(level int) {
	Error = log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	if level >= LevelInfo {
		Info = log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime)
	} else {
		Info = log.New(io.Discard, "", 0)
	}

	if level >= LevelDebug {
		Debug = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
	} else {
		Debug = log.New(io.Discard, "", 0)
	}
}

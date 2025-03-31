package logger

import (
	"io"
	"log"
	"os"
)

const (
	LevelError = iota
	LevelWarning
	LevelInfo
	LevelDebug
)

var (
	ErrorLogger *log.Logger
	WarnLogger *log.Logger
	InfoLogger *log.Logger
	DebugLogger *log.Logger

	currentLevel = LevelInfo
)

func Setup(level int){
	currentLevel = level

	ErrorLogger = log.New(os.Stderr, "Error: ", log.Ldate|log.Ltime|log.Lshortfile)

	if level >= LevelWarning {
		WarnLogger = log.New(os.Stderr, "Warning: ", log.Ldate|log.Ltime)
	} else {
		WarnLogger = log.New(io.Discard, "", 0)
	}

	if level >= LevelDebug {
        DebugLogger = log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
    } else {
        DebugLogger = log.New(io.Discard, "", 0)
    }
}

func GetCurrentLevel() int {
	return currentLevel
}

func SetLevel(newLevel int)  {
	currentLevel = newLevel

	Setup(newLevel)
}
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


const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorGreen  = "\033[32m"
	ColorBlue   = "\033[36m" 
	ColorPurple = "\033[35m"
	ColorBold   = "\033[1m"
)

var (
	ErrorLogger  *log.Logger
	WarnLogger   *log.Logger
	InfoLogger   *log.Logger
	DebugLogger  *log.Logger
	currentLevel = LevelInfo
	useColors    = true 
)


func Setup(level int) {
	currentLevel = level

	
	errorPrefix := "ERROR: "
	if useColors {
		errorPrefix = ColorRed + ColorBold + errorPrefix + ColorReset
	}
	ErrorLogger = log.New(os.Stderr, errorPrefix, log.Ldate|log.Ltime|log.Lshortfile)

	
	warningPrefix := "WARNING: "
	if useColors {
		warningPrefix = ColorYellow + warningPrefix + ColorReset
	}
	if level >= LevelWarning {
		WarnLogger = log.New(os.Stderr, warningPrefix, log.Ldate|log.Ltime)
	} else {
		WarnLogger = log.New(io.Discard, "", 0)
	}

	
	infoPrefix := "INFO: "
	if useColors {
		infoPrefix = ColorGreen + infoPrefix + ColorReset
	}
	if level >= LevelInfo {
		InfoLogger = log.New(os.Stdout, infoPrefix, log.Ldate|log.Ltime)
	} else {
		InfoLogger = log.New(io.Discard, "", 0)
	}

	
	debugPrefix := "DEBUG: "
	if useColors {
		debugPrefix = ColorBlue + debugPrefix + ColorReset
	}
	if level >= LevelDebug {
		DebugLogger = log.New(os.Stdout, debugPrefix, log.Ldate|log.Ltime|log.Lshortfile)
	} else {
		DebugLogger = log.New(io.Discard, "", 0)
	}
}


func SetColors(enabled bool) {
	if useColors != enabled {
		useColors = enabled
		Setup(currentLevel)
	}
}


func GetCurrentLevel() int {
	return currentLevel
}


func SetLevel(newLevel int) {
	currentLevel = newLevel
	Setup(newLevel)
}

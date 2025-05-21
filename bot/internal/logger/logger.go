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

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorGreen  = "\033[32m"
	ColorBlue   = "\033[36m" // Cyan, easier to read than pure blue
	ColorPurple = "\033[35m"
	ColorBold   = "\033[1m"
)

var (
	ErrorLogger  *log.Logger
	WarnLogger   *log.Logger
	InfoLogger   *log.Logger
	DebugLogger  *log.Logger
	currentLevel = LevelInfo
	useColors    = true // Can be toggled if needed
)

// Setup initializes the loggers with the specified level
func Setup(level int) {
	currentLevel = level

	// Error logger - Red
	errorPrefix := "ERROR: "
	if useColors {
		errorPrefix = ColorRed + ColorBold + errorPrefix + ColorReset
	}
	ErrorLogger = log.New(os.Stderr, errorPrefix, log.Ldate|log.Ltime|log.Lshortfile)

	// Warning logger - Yellow
	warningPrefix := "WARNING: "
	if useColors {
		warningPrefix = ColorYellow + warningPrefix + ColorReset
	}
	if level >= LevelWarning {
		WarnLogger = log.New(os.Stderr, warningPrefix, log.Ldate|log.Ltime)
	} else {
		WarnLogger = log.New(io.Discard, "", 0)
	}

	// Info logger - Green
	infoPrefix := "INFO: "
	if useColors {
		infoPrefix = ColorGreen + infoPrefix + ColorReset
	}
	if level >= LevelInfo {
		InfoLogger = log.New(os.Stdout, infoPrefix, log.Ldate|log.Ltime)
	} else {
		InfoLogger = log.New(io.Discard, "", 0)
	}

	// Debug logger - Cyan
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

// SetColors enables or disables color output
func SetColors(enabled bool) {
	if useColors != enabled {
		useColors = enabled
		Setup(currentLevel)
	}
}

// GetCurrentLevel returns the current logging level
func GetCurrentLevel() int {
	return currentLevel
}

// SetLevel changes the current logging level
func SetLevel(newLevel int) {
	currentLevel = newLevel
	Setup(newLevel)
}

import sys
import time

class ColoredLogger:
    RESET = "\033[0m"
    BOLD = "\033[1m"
    RED = "\033[31m"
    GREEN = "\033[32m"
    YELLOW = "\033[33m"
    BLUE = "\033[34m"
    CYAN = "\033[36m"
    PURPLE = "\033[35m"
    GRAY = "\033[90m"

    ERROR = 0
    WARNING = 1
    INFO = 2
    DEBUG = 3

    def __init__(self, level=INFO, use_colors=True):
        self.level = level
        self.use_colors = use_colors
        
    def format_timestamp(self):
        timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
        return timestamp
        
    def error(self, message):
        if self.level >= self.ERROR:
            prefix = "ERROR: "
            if self.use_colors:
                prefix = f"{self.RED}{self.BOLD}{prefix}{self.RESET}"
            timestamp = self.format_timestamp()
            print(f"{prefix}{timestamp} {message}", file=sys.stderr)
            
    def warning(self, message):
        if self.level >= self.WARNING:
            prefix = "WARNING: "
            if self.use_colors:
                prefix = f"{self.YELLOW}{self.BOLD}{prefix}{self.RESET}"
            timestamp = self.format_timestamp()
            print(f"{prefix}{timestamp} {message}", file=sys.stderr)
            
    def info(self, message):
        if self.level >= self.INFO:
            prefix = "INFO: "
            if self.use_colors:
                prefix = f"{self.GREEN}{prefix}{self.RESET}"
            timestamp = self.format_timestamp()
            print(f"{prefix}{timestamp} {message}")
            
    def debug(self, message):
        if self.level >= self.DEBUG:
            prefix = "DEBUG: "
            if self.use_colors:
                prefix = f"{self.CYAN}{prefix}{self.RESET}"
            timestamp = self.format_timestamp()
            print(f"{prefix}{timestamp} {message}")

logger = ColoredLogger(level=ColoredLogger.INFO)

def set_level(level):
    logger.level = level
    
def set_colors(use_colors):
    logger.use_colors = use_colors
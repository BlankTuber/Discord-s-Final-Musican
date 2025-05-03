import json
import os
import signal
import time
import ytdlp_handler
import uds_handler
import logger

def load_config():
    config = {
        "download_path": "../../shared/",
        "uds_link": "/tmp/downloader.sock",
        "allowed_origins": ["https://youtube.com"]
    }
    
    config_path = "../config/config.json"
    try:
        with open(config_path, "r") as file:
            data = json.load(file)
            config.update(data)
    except (FileNotFoundError, json.JSONDecodeError) as e:
        logger.logger.error(f"Config error, using defaults: {e}")
    
    config["download_path"] = os.path.abspath(os.path.expanduser(config["download_path"]))
    # Use the download path directly instead of its parent directory
    config["db_path"] = os.path.join(config["download_path"], "musicbot.db")
    
    os.makedirs(config["download_path"], exist_ok=True)
    logger.logger.info(f"Download directory ready: {config['download_path']}")
    logger.logger.info(f"Database path: {config['db_path']}")
    
    return config

def ensure_database_exists(db_path):
    if not os.path.exists(db_path):
        logger.logger.warning(f"Database file not found at: {db_path}")
        logger.logger.warning("Please run the database initializer before starting the downloader")
        logger.logger.warning("Command: cd ../shared && go build -o db_init db_initializer.go && ./db_init -path .")
        return False
    return True

def main():
    config = load_config()
    logger.logger.info("Reading configuration...")
    logger.logger.info(f"Downloading files to: {config['download_path']}")
    logger.logger.info(f"Using socket at: {config['uds_link']}")
    logger.logger.info(f"Allowed origins: {config['allowed_origins']}")
    
    db_exists = ensure_database_exists(config["db_path"])
    if not db_exists:
        logger.logger.warning("Database not found, continuing with service startup but some functionality may be limited")
    
    ytdlp_handler.initialize(config)
    uds_handler.initialize(config)
    
    if uds_handler.start_server():
        logger.logger.info(f"UDS server listening on {config['uds_link']}")
    else:
        logger.logger.error("Failed to start UDS server")
    
    def shutdown_handler(sig, frame):
        logger.logger.info("\nShutting down gracefully...")
        uds_handler.stop_server()
        logger.logger.info("Goodbye!")
        exit(0)
    
    signal.signal(signal.SIGINT, shutdown_handler)
    signal.signal(signal.SIGTERM, shutdown_handler)
    
    logger.logger.info("\nServer is running. Press Ctrl+C to exit.")
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        pass
    finally:
        shutdown_handler(None, None)

if __name__ == "__main__":
    main()

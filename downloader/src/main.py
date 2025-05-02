import json
import os
import signal
import time
import ytdlp_handler
import uds_handler

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
        print(f"Config error, using defaults: {e}")
    
    config["download_path"] = os.path.abspath(os.path.expanduser(config["download_path"]))
    # Fix: Use the download path directly instead of its parent directory
    config["db_path"] = os.path.join(config["download_path"], "musicbot.db")
    
    os.makedirs(config["download_path"], exist_ok=True)
    print(f"Download directory ready: {config['download_path']}")
    print(f"Database path: {config['db_path']}")
    
    return config

def ensure_database_exists(db_path):
    if not os.path.exists(db_path):
        print(f"Warning: Database file not found at: {db_path}")
        print("Please run the database initializer before starting the downloader")
        return False
    return True

def main():
    config = load_config()
    print("Reading configuration...")
    print(f"Downloading files to: {config['download_path']}")
    print(f"Using socket at: {config['uds_link']}")
    print(f"Allowed origins: {config['allowed_origins']}")
    
    if not ensure_database_exists(config["db_path"]):
        print("Database not found, but continuing with service startup")
    
    ytdlp_handler.initialize(config)
    uds_handler.initialize(config)
    
    if uds_handler.start_server():
        print(f"UDS server listening on {config['uds_link']}")
    else:
        print("Failed to start UDS server")
    
    def shutdown_handler(sig, frame):
        print("\nShutting down gracefully...")
        uds_handler.stop_server()
        print("Goodbye!")
        exit(0)
    
    signal.signal(signal.SIGINT, shutdown_handler)
    signal.signal(signal.SIGTERM, shutdown_handler)
    
    print("\nServer is running. Press Ctrl+C to exit.")
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        pass
    finally:
        shutdown_handler(None, None)

if __name__ == "__main__":
    main()
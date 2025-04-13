# config.py
import json
import os

DOWNLOAD_PATH = "../../shared/"
UDS_LINK = "/tmp/downloader.sock"
ALLOWED_ORIGINS = ["https://youtube.com"]

def load_config():
    global DOWNLOAD_PATH, UDS_LINK, ALLOWED_ORIGINS
    
    config_path = "../config/config.json"
    try:
        with open(config_path, "r") as file:
            data = json.load(file)
            
            DOWNLOAD_PATH = data.get("download_path", DOWNLOAD_PATH)
            UDS_LINK = data.get("uds_link", UDS_LINK)
            ALLOWED_ORIGINS = data.get("allowed_origins", ALLOWED_ORIGINS)
            
            ensure_download_directory()
            
            return data
    except FileNotFoundError:
        print(f"Config file not found at {config_path}, using default values")
        return {"message": "Using default configuration"}
    except json.JSONDecodeError:
        print(f"Invalid JSON in config file at {config_path}, using default values")
        return {"message": "Using default configuration"}


def ensure_download_directory():
    try:
        os.makedirs(DOWNLOAD_PATH, exist_ok=True)
        print(f"Download directory ready: {DOWNLOAD_PATH}")
    except PermissionError:
        print(f"Error: No permission to create download directory at {DOWNLOAD_PATH}")
    except Exception as e:
        print(f"Error creating download directory: {e}")

config_data = load_config()
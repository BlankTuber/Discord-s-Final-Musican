import json
import os
import ytdlp_handler

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
    
    os.makedirs(config["download_path"], exist_ok=True)
    print(f"Download directory ready: {config['download_path']}")
    
    return config

def main():
    config = load_config()
    print("Reading configuration...")
    print(f"Downloading files to: {config['download_path']}")
    print(f"Using socket at: {config['uds_link']}")
    print(f"Allowed origins: {config['allowed_origins']}")
    
    ytdlp_handler.initialize(config)
    
    print("\n--- Testing single audio download ---")
    test_url = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
    print(f"Downloading: {test_url}")
    result = ytdlp_handler.download_audio(test_url, max_duration_seconds=600, max_size_mb=50)
    
    if result:
        if result.get('skipped'):
            print(f"Download skipped - file already exists!")
        else:
            print(f"Download successful!")
        
        print(f"Title: {result['title']}")
        print(f"Saved as: {result['filename']}")
        print(f"Duration: {result['duration']} seconds")
    else:
        print("Download failed or was skipped")
    
    print("\n--- Testing search functionality ---")
    search_term = "lofi beats"
    print(f"Searching for: {search_term}")
    search_results = ytdlp_handler.search(search_term, limit=3)
    
    if search_results:
        print(f"Found {len(search_results)} results:")
        for i, result in enumerate(search_results, 1):
            print(f"{i}. {result['title']} by {result['uploader']}")
            print(f"   URL: {result['url']}")
            print(f"   Duration: {result['duration']} seconds")
            print()
    else:
        print("Search returned no results")

if __name__ == "__main__":
    main()
from ytdlp import audio, playlist, search as search_module, utils
import os
from database import Database

config = {}
db = None

def initialize(cfg):
    global config, db
    config.update(cfg)
    utils.init(config)
    
    # Use the db_path directly from config instead of calculating it
    db_path = config.get("db_path")
    if not db_path:
        # Fallback only if db_path is not provided
        db_path = os.path.join(os.path.dirname(config["download_path"]), "musicbot.db")
        print(f"Warning: db_path not provided in config, using default: {db_path}")
    
    db = Database(db_path)

def download_audio(url, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    result = audio.download(
        url, 
        config["download_path"], 
        db,
        max_duration_seconds=max_duration_seconds, 
        max_size_mb=max_size_mb, 
        allow_live=allow_live
    )
    
    if not result:
        return {"status": "error", "message": "Download failed"}
    
    return {"status": "success"}

def download_playlist(url, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    result = playlist.download(
        url, 
        config["download_path"], 
        db,
        max_items=max_items,
        max_duration_seconds=max_duration_seconds, 
        max_size_mb=max_size_mb, 
        allow_live=allow_live
    )
    
    if not result:
        return {"status": "error", "message": "Playlist download failed"}
    
    return {"status": "success", "count": result.get("successful_downloads", 0)}

def search(query, platform='youtube', limit=5, include_live=False):
    platform_lower = platform.lower()
    allowed_platform = None
    
    if platform_lower in ['youtube', 'youtu.be', 'youtube.com', 'https://youtube.com', 'https://youtu.be']:
        allowed_platform = 'https://youtube.com'
    elif platform_lower in ['soundcloud', 'soundcloud.com', 'https://soundcloud.com']:
        allowed_platform = 'https://soundcloud.com'
    elif platform_lower in ['music.youtube.com', 'ytmusic', 'youtube music', 'https://music.youtube.com']:
        allowed_platform = 'https://music.youtube.com'
    else:
        allowed_platform = utils.get_platform(platform)
    
    if allowed_platform not in config["allowed_origins"]:
        print(f"Platform '{allowed_platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{allowed_platform}' is not allowed"}
    
    results = search_module.find(
        query, 
        platform=platform, 
        limit=limit, 
        include_live=include_live
    )
    
    if not results:
        return {"results": []}
        
    return {"results": results}
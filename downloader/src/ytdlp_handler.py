from ytdlp import audio, playlist, search as search_module, utils
import os
import time
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
    
    if not os.path.exists(db_path):
        print(f"Warning: Database file does not exist at {db_path}")
        print("Please run the database initializer before starting the downloader")
    
    db = Database(db_path)
    print(f"Connected to database at: {db_path}")

def download_audio(url, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        # First check if the song exists in the database and the file exists
        song = db.get_song_by_url(url)
        if song and os.path.exists(song['file_path']):
            print(f"Song already exists in database and file exists: {song['title']}")
            return {
                "status": "success",
                "title": song['title'],
                "filename": song['file_path'],
                "duration": song['duration'],
                "file_size": song['file_size'],
                "platform": song['platform'],
                "artist": song.get('artist', ''),
                "thumbnail_url": song.get('thumbnail_url', ''),
                "is_stream": song.get('is_stream', False),
                "skipped": True
            }
    except Exception as e:
        print(f"Error checking database: {e}")
    
    try:
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
        
        # Double check that the song is now in the database
        try:
            song = db.get_song_by_url(url)
            if song:
                return {
                    "status": "success",
                    "title": song['title'],
                    "filename": song['file_path'],
                    "duration": song['duration'],
                    "file_size": song['file_size'],
                    "platform": song['platform'],
                    "artist": song.get('artist', ''),
                    "thumbnail_url": song.get('thumbnail_url', ''),
                    "is_stream": song.get('is_stream', False),
                    "id": song['id'],
                    "skipped": False
                }
        except Exception as e:
            print(f"Error getting song from database after download: {e}")
        
        # If we can't get the song from the database, return the result directly
        return {
            "status": "success",
            "title": result.get('title', 'Unknown'),
            "filename": result.get('filename', ''),
            "duration": result.get('duration'),
            "file_size": result.get('file_size'),
            "platform": result.get('platform', platform),
            "id": result.get('id'),
            "skipped": result.get('skipped', False)
        }
    except Exception as e:
        print(f"Error in download_audio: {e}")
        return {"status": "error", "message": str(e)}

def download_playlist(url, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        # Check if playlist already exists in database
        try:
            db_playlist = db.get_playlist_by_url(url)
            if db_playlist:
                print(f"Playlist already exists in database: {db_playlist['title']}")
                # We still need to download the playlist to get any new songs
        except Exception as e:
            print(f"Error checking playlist in database: {e}")
            
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
        
        # Make sure all downloads are complete before returning
        time.sleep(1)
        
        return {
            "status": "success", 
            "count": result.get("count", 0),
            "successful_downloads": result.get("successful_downloads", 0),
            "playlist_title": result.get("playlist_title", "Unknown Playlist"),
            "playlist_url": result.get("playlist_url", url),
            "items": result.get("items", [])
        }
    except Exception as e:
        print(f"Error in download_playlist: {e}")
        return {"status": "error", "message": str(e)}

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
    
    try:
        results = search_module.find(
            query, 
            platform=platform, 
            limit=limit, 
            include_live=include_live
        )
        
        if not results:
            return {"results": []}
            
        return {"results": results}
    except Exception as e:
        print(f"Error in search: {e}")
        return {"status": "error", "message": str(e)}
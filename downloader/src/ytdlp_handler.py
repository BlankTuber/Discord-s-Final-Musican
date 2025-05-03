from ytdlp import audio, playlist, search as search_module, utils
import os
import time
import traceback
from database import Database

config = {}
db = None

def initialize(cfg):
    global config, db
    config.update(cfg)
    utils.init(config)
    
    db_path = config.get("db_path")
    if not db_path:
        db_path = os.path.join(os.path.dirname(config["download_path"]), "musicbot.db")
        print(f"Warning: db_path not provided in config, using default: {db_path}")
    
    if not os.path.exists(db_path):
        print(f"Warning: Database file does not exist at {db_path}")
        print("Please run the database initializer before starting the downloader")
    
    db = Database(db_path)
    print(f"Connected to database at: {db_path}")

def download_audio(url, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    print(f"HANDLER: Starting download_audio for URL: {url}")
    start_time = time.time()
    
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"HANDLER: Platform '{platform}' is not in the allowed origins list.")
        print(f"HANDLER: Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        print("HANDLER: Checking database for existing song")
        song = db.get_song_by_url(url)
        if song and os.path.exists(song['file_path']):
            print(f"HANDLER: Song already exists in database and file exists: {song['title']}")
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
        print(f"HANDLER: Error checking database: {e}")
        print(f"HANDLER: Traceback: {traceback.format_exc()}")
    
    try:
        print(f"HANDLER: Starting audio download with params: max_duration={max_duration_seconds}, max_size={max_size_mb}")
        result = audio.download(
            url, 
            config["download_path"], 
            db,
            max_duration_seconds=max_duration_seconds, 
            max_size_mb=max_size_mb, 
            allow_live=allow_live
        )
        
        elapsed = time.time() - start_time
        
        if not result:
            print(f"HANDLER: Download failed after {elapsed:.2f} seconds")
            return {"status": "error", "message": "Download failed"}
        
        print(f"HANDLER: Download succeeded in {elapsed:.2f} seconds, checking database for song record")
        
        try:
            song = db.get_song_by_url(url)
            if song:
                print(f"HANDLER: Song record found in database: {song['title']}")
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
            print(f"HANDLER: Error getting song from database after download: {e}")
            print(f"HANDLER: Traceback: {traceback.format_exc()}")
        
        print(f"HANDLER: Returning download result directly: {result.get('title', 'Unknown')}")
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
        elapsed = time.time() - start_time
        print(f"HANDLER: Error in download_audio after {elapsed:.2f} seconds: {e}")
        print(f"HANDLER: Traceback: {traceback.format_exc()}")
        return {"status": "error", "message": str(e)}

def download_playlist(url, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    print(f"HANDLER: Starting download_playlist for URL: {url}, max_items: {max_items}")
    start_time = time.time()
    
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"HANDLER: Platform '{platform}' is not in the allowed origins list.")
        print(f"HANDLER: Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        try:
            print("HANDLER: Checking database for existing playlist")
            db_playlist = db.get_playlist_by_url(url)
            if db_playlist:
                print(f"HANDLER: Playlist already exists in database: {db_playlist['title']}")
        except Exception as e:
            print(f"HANDLER: Error checking playlist in database: {e}")
            print(f"HANDLER: Traceback: {traceback.format_exc()}")
            
        print(f"HANDLER: Starting playlist download with params: max_items={max_items}, max_duration={max_duration_seconds}, max_size={max_size_mb}")
        result = playlist.download(
            url, 
            config["download_path"], 
            db,
            max_items=max_items,
            max_duration_seconds=max_duration_seconds, 
            max_size_mb=max_size_mb, 
            allow_live=allow_live
        )
        
        elapsed = time.time() - start_time
        
        if not result:
            print(f"HANDLER: Playlist download failed after {elapsed:.2f} seconds")
            return {"status": "error", "message": "Playlist download failed"}
        
        item_count = result.get("count", 0)
        successful = result.get("successful_downloads", 0)
        first_track = result.get("first_track")
        
        print(f"HANDLER: Playlist download completed in {elapsed:.2f} seconds")
        print(f"HANDLER: Items: {item_count}, Successfully downloaded: {successful}")
        if first_track:
            print(f"HANDLER: First track: {first_track.get('title', 'Unknown')}")
        
        return {
            "status": "success", 
            "count": item_count,
            "successful_downloads": successful,
            "playlist_title": result.get("playlist_title", "Unknown Playlist"),
            "playlist_url": result.get("playlist_url", url),
            "items": result.get("items", []),
            "first_track": first_track
        }
    except Exception as e:
        elapsed = time.time() - start_time
        print(f"HANDLER: Error in download_playlist after {elapsed:.2f} seconds: {e}")
        print(f"HANDLER: Traceback: {traceback.format_exc()}")
        return {"status": "error", "message": str(e)}

def search(query, platform='youtube', limit=5, include_live=False):
    print(f"HANDLER: Starting search for '{query}' on platform '{platform}', limit: {limit}")
    start_time = time.time()
    
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
        print(f"HANDLER: Platform '{allowed_platform}' is not in the allowed origins list.")
        print(f"HANDLER: Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{allowed_platform}' is not allowed"}
    
    try:
        print(f"HANDLER: Calling search module with query '{query}', platform '{platform}', limit {limit}")
        results = search_module.find(
            query, 
            platform=platform, 
            limit=limit, 
            include_live=include_live
        )
        
        elapsed = time.time() - start_time
        
        if not results:
            print(f"HANDLER: No search results found after {elapsed:.2f} seconds")
            return {"results": []}
        
        result_count = len(results)    
        print(f"HANDLER: Search completed in {elapsed:.2f} seconds, found {result_count} results")
        
        return {"results": results}
    except Exception as e:
        elapsed = time.time() - start_time
        print(f"HANDLER: Error in search after {elapsed:.2f} seconds: {e}")
        print(f"HANDLER: Traceback: {traceback.format_exc()}")
        return {"status": "error", "message": str(e)}
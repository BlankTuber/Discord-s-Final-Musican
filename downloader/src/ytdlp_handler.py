from ytdlp import audio, playlist, search as search_module, utils
import os
import time
import traceback
from database import Database
import logger

config = {}
db = None

def initialize(cfg):
    global config, db
    config.update(cfg)
    utils.init(config)
    
    db_path = config.get("db_path")
    if not db_path:
        db_path = os.path.join(os.path.dirname(config["download_path"]), "musicbot.db")
        logger.logger.warning(f"db_path not provided in config, using default: {db_path}")
    
    if not os.path.exists(db_path):
        logger.logger.warning(f"Database file does not exist at {db_path}")
        logger.logger.warning("Please run the database initializer before starting the downloader")
    
    db = Database(db_path)
    logger.logger.info(f"Connected to database at: {db_path}")

def download_audio(url, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    logger.logger.info(f"Starting download_audio for URL: {url}")
    start_time = time.time()
    
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        logger.logger.warning(f"Platform '{platform}' is not in the allowed origins list.")
        logger.logger.warning(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        logger.logger.info("Checking database for existing song")
        song = db.get_song_by_url(url)
        if song and os.path.exists(song['file_path']):
            logger.logger.info(f"Song already exists in database and file exists: {song['title']}")
            artist = song['artist'] if 'artist' in song else ''
            thumbnail_url = song['thumbnail_url'] if 'thumbnail_url' in song else ''
            is_stream = bool(song['is_stream']) if 'is_stream' in song else False
            
            return {
                "status": "success",
                "title": song['title'],
                "filename": song['file_path'],
                "duration": song['duration'],
                "file_size": song['file_size'],
                "platform": song['platform'],
                "artist": artist,
                "thumbnail_url": thumbnail_url,
                "is_stream": is_stream,
                "id": song['id'],
                "skipped": True
            }
    except Exception as e:
        logger.logger.error(f"Error checking database: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
    
    try:
        logger.logger.info(f"Starting audio download with params: max_duration={max_duration_seconds}, max_size={max_size_mb}")
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
            logger.logger.error(f"Download failed after {elapsed:.2f} seconds")
            return {"status": "error", "message": "Download failed"}
        
        logger.logger.info(f"Download succeeded in {elapsed:.2f} seconds, checking database for song record")
        
        try:
            song = db.get_song_by_url(url)
            if song:
                logger.logger.info(f"Song record found in database: {song['title']}")
                artist = song['artist'] if 'artist' in song else ''
                thumbnail_url = song['thumbnail_url'] if 'thumbnail_url' in song else ''
                is_stream = bool(song['is_stream']) if 'is_stream' in song else False
                
                return {
                    "status": "success",
                    "title": song['title'],
                    "filename": song['file_path'],
                    "duration": song['duration'],
                    "file_size": song['file_size'],
                    "platform": song['platform'],
                    "artist": artist,
                    "thumbnail_url": thumbnail_url,
                    "is_stream": is_stream,
                    "id": song['id'],
                    "skipped": False
                }
        except Exception as e:
            logger.logger.error(f"Error getting song from database after download: {e}")
            logger.logger.debug(f"Traceback: {traceback.format_exc()}")
        
        logger.logger.info(f"Returning download result directly: {result.get('title', 'Unknown')}")
        return {
            "status": "success",
            "title": result.get('title', 'Unknown'),
            "filename": result.get('filename', ''),
            "duration": result.get('duration'),
            "file_size": result.get('file_size'),
            "platform": result.get('platform', platform),
            "artist": result.get('artist', ''),
            "thumbnail_url": result.get('thumbnail_url', ''),
            "is_stream": result.get('is_stream', False),
            "id": result.get('id'),
            "skipped": result.get('skipped', False)
        }
    except Exception as e:
        elapsed = time.time() - start_time
        logger.logger.error(f"Error in download_audio after {elapsed:.2f} seconds: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
        return {"status": "error", "message": str(e)}

def download_playlist(url, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    logger.logger.info(f"Starting download_playlist for URL: {url}, max_items: {max_items}")
    start_time = time.time()
    
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        logger.logger.warning(f"Platform '{platform}' is not in the allowed origins list.")
        logger.logger.warning(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        try:
            logger.logger.info("Checking database for existing playlist")
            db_playlist = db.get_playlist_by_url(url)
            if db_playlist:
                logger.logger.info(f"Playlist already exists in database: {db_playlist['title']}")
        except Exception as e:
            logger.logger.error(f"Error checking playlist in database: {e}")
            logger.logger.debug(f"Traceback: {traceback.format_exc()}")
            
        logger.logger.info(f"Starting playlist download with params: max_items={max_items}, max_duration={max_duration_seconds}, max_size={max_size_mb}")
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
            logger.logger.error(f"Playlist download failed after {elapsed:.2f} seconds")
            return {"status": "error", "message": "Playlist download failed"}
        
        item_count = result.get("count", 0)
        successful = result.get("successful_downloads", 0)
        first_track = result.get("first_track")
        
        logger.logger.info(f"Playlist download completed in {elapsed:.2f} seconds")
        logger.logger.info(f"Items: {item_count}, Successfully downloaded: {successful}")
        if first_track:
            logger.logger.info(f"First track: {first_track.get('title', 'Unknown')}")
        
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
        logger.logger.error(f"Error in download_playlist after {elapsed:.2f} seconds: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
        return {"status": "error", "message": str(e)}

def search(query, platform='youtube', limit=5, include_live=False):
    logger.logger.info(f"Starting search for '{query}' on platform '{platform}', limit: {limit}")
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
        logger.logger.warning(f"Platform '{allowed_platform}' is not in the allowed origins list.")
        logger.logger.warning(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{allowed_platform}' is not allowed"}
    
    try:
        logger.logger.info(f"Calling search module with query '{query}', platform '{platform}', limit {limit}")
        results = search_module.find(
            query, 
            platform=platform, 
            limit=limit, 
            include_live=include_live
        )
        
        elapsed = time.time() - start_time
        
        if not results:
            logger.logger.info(f"No search results found after {elapsed:.2f} seconds")
            return {"results": []}
        
        result_count = len(results)    
        logger.logger.info(f"Search completed in {elapsed:.2f} seconds, found {result_count} results")
        
        return {"results": results}
    except Exception as e:
        elapsed = time.time() - start_time
        logger.logger.error(f"Error in search after {elapsed:.2f} seconds: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
        return {"status": "error", "message": str(e)}
from ytdlp import audio, playlist, search as search_module, utils, streaming
import os
import time
import traceback
from database import Database
import logger
import yt_dlp

config = {}
db = None
event_callbacks = []

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

def register_event_callback(callback):
    """Register a callback function to be called when events occur"""
    global event_callbacks
    if callback not in event_callbacks:
        event_callbacks.append(callback)
        return True
    return False

def fire_event(event_type, event_data):
    """Fire an event to all registered callbacks"""
    global event_callbacks
    for callback in event_callbacks:
        try:
            callback(event_type, event_data)
        except Exception as e:
            logger.logger.error(f"Error in event callback: {e}")
            logger.logger.debug(f"Traceback: {traceback.format_exc()}")

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

def download_playlist(url, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False, requester=None, guild_id=None):
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
            
        logger.logger.info(f"Starting streaming playlist download with params: max_items={max_items}, max_duration={max_duration_seconds}, max_size={max_size_mb}")
        
        # Use the streaming download method which fires events
        result = streaming.download_playlist_streaming(
            url, 
            config["download_path"], 
            db,
            event_callback=fire_event,  # Pass our event firing function
            requester=requester,
            guild_id=guild_id,
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

def get_playlist_info(url, max_items=None):
    """Get information about a playlist without downloading it"""
    logger.logger.info(f"Getting playlist info for URL: {url}, max_items: {max_items}")
    start_time = time.time()
    
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        logger.logger.warning(f"Platform '{platform}' is not in the allowed origins list.")
        logger.logger.warning(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        with yt_dlp.YoutubeDL({
            'skip_download': True, 
            'quiet': True, 
            'noplaylist': False,
            'extract_flat': True,
            'socket_timeout': 30,
            'ignoreerrors': True
        }) as ydl:
            info = ydl.extract_info(url, download=False)
            
            if not info:
                return {"status": "error", "message": "Could not extract playlist info"}
            
            # Check if this is a playlist or a single video
            if 'entries' not in info:
                # This is a single video, not a playlist
                return {
                    "status": "success",
                    "playlist_title": info.get('title', 'Single Video'),
                    "playlist_url": url,
                    "total_tracks": 1,
                    "is_playlist": False
                }
            
            entries = list(info.get('entries', []))
            
            # Filter out unavailable videos
            filtered_entries = []
            for entry in entries:
                if entry and entry.get('id') is not None:
                    filtered_entries.append(entry)
                else:
                    logger.logger.info(f"Skipping unavailable video in playlist")
            
            entries = filtered_entries
            
            if max_items:
                entries = entries[:max_items]
            
            total_tracks = len(entries)
            
            playlist_title = info.get('title', 'Unknown Playlist')
            
            elapsed = time.time() - start_time
            logger.logger.info(f"Playlist info retrieved in {elapsed:.2f} seconds")
            logger.logger.info(f"Playlist title: {playlist_title}, total tracks: {total_tracks}")
            
            return {
                "status": "success",
                "playlist_title": playlist_title,
                "playlist_url": url,
                "total_tracks": total_tracks,
                "is_playlist": True
            }
    except Exception as e:
        elapsed = time.time() - start_time
        logger.logger.error(f"Error getting playlist info after {elapsed:.2f} seconds: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
        return {"status": "error", "message": str(e)}

def download_playlist_item(url, index, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    """Download a specific item from a playlist by index"""
    logger.logger.info(f"Downloading playlist item {index} from URL: {url}")
    start_time = time.time()
    
    platform = utils.get_platform(url)
    platform_prefix = utils.get_platform_prefix(platform)
    
    if platform not in config["allowed_origins"]:
        logger.logger.warning(f"Platform '{platform}' is not in the allowed origins list.")
        logger.logger.warning(f"Allowed origins: {config['allowed_origins']}")
        return {"status": "error", "message": f"Platform '{platform}' is not allowed"}
    
    try:
        # First, extract the video URL from the playlist
        with yt_dlp.YoutubeDL({
            'skip_download': True, 
            'quiet': True, 
            'noplaylist': False,
            'extract_flat': True,
            'playliststart': index + 1,  # 1-based index
            'playlistend': index + 1,
            'socket_timeout': 30,
            'ignoreerrors': True
        }) as ydl:
            info = ydl.extract_info(url, download=False)
            
            if not info or 'entries' not in info or not info['entries']:
                return {"status": "error", "message": f"Could not find item at index {index}"}
            
            entries = list(info.get('entries', []))
            
            if not entries:
                return {"status": "error", "message": f"No valid entries found at index {index}"}
            
            entry = entries[0]
            if not entry or entry.get('id') is None:
                return {"status": "error", "message": f"Item at index {index} is unavailable"}
            
            video_id = entry.get('id')
            video_title = entry.get('title', f'Unknown Track {index}')
            video_url = entry.get('url', f"https://www.youtube.com/watch?v={video_id}")
            
            # Now download the individual video
            logger.logger.info(f"Downloading playlist item: {video_title} ({video_url})")
            
            result = audio.download(
                video_url, 
                config["download_path"], 
                db,
                max_duration_seconds=max_duration_seconds, 
                max_size_mb=max_size_mb, 
                allow_live=allow_live
            )
            
            elapsed = time.time() - start_time
            
            if not result:
                logger.logger.error(f"Download failed for playlist item after {elapsed:.2f} seconds")
                return {"status": "error", "message": f"Download failed for item {index}"}
            
            logger.logger.info(f"Playlist item download completed in {elapsed:.2f} seconds")
            logger.logger.info(f"Downloaded: {result.get('title', 'Unknown')}")
            
            # Add to playlist in database if needed
            try:
                db_playlist = db.get_playlist_by_url(url)
                
                if db_playlist and 'id' in result:
                    song_id = result['id']
                    try:
                        db.add_song_to_playlist(db_playlist['id'], song_id, index)
                        logger.logger.info(f"Added song ID {song_id} to playlist ID {db_playlist['id']}")
                    except Exception as e:
                        logger.logger.error(f"Error adding song to playlist: {e}")
            except Exception as e:
                logger.logger.error(f"Error handling playlist database operations: {e}")
            
            return {
                "status": "success",
                "title": result.get('title', video_title),
                "filename": result.get('filename', ''),
                "duration": result.get('duration'),
                "file_size": result.get('file_size'),
                "platform": result.get('platform', platform),
                "artist": result.get('artist', ''),
                "thumbnail_url": result.get('thumbnail_url', ''),
                "is_stream": result.get('is_stream', False),
                "id": result.get('id'),
                "index": index
            }
    except Exception as e:
        elapsed = time.time() - start_time
        logger.logger.error(f"Error in download_playlist_item after {elapsed:.2f} seconds: {e}")
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

def start_playlist_download(url, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False, requester=None, guild_id=None):
    """
    Start a playlist download in a separate thread, returning a playlist ID
    so the client can check for updates
    """
    logger.logger.info(f"Starting async playlist download for URL: {url}, max_items: {max_items}")
    
    # Generate a unique playlist ID
    import uuid
    playlist_id = str(uuid.uuid4())
    
    # Start the download in a background thread
    import threading
    thread = threading.Thread(
        target=_background_playlist_download,
        args=(playlist_id, url, max_items, max_duration_seconds, max_size_mb, allow_live, requester, guild_id)
    )
    thread.daemon = True
    thread.start()
    
    # Get initial playlist info for the response
    playlist_info = get_playlist_info(url, max_items)
    if playlist_info.get("status") == "error":
        return playlist_info
    
    return {
        "status": "success",
        "playlist_id": playlist_id,
        "playlist_title": playlist_info.get("playlist_title", "Unknown Playlist"),
        "total_tracks": playlist_info.get("total_tracks", 0),
        "is_playlist": playlist_info.get("is_playlist", True)
    }

def _background_playlist_download(playlist_id, url, max_items, max_duration_seconds, max_size_mb, allow_live, requester, guild_id):
    """Background thread to download a playlist and send events"""
    try:
        result = download_playlist(
            url, 
            max_items=max_items,
            max_duration_seconds=max_duration_seconds, 
            max_size_mb=max_size_mb, 
            allow_live=allow_live,
            requester=requester,
            guild_id=guild_id
        )
        
        # Send a final event when the playlist is complete
        fire_event("playlist_download_completed", {
            "playlist_id": playlist_id,
            "result": result
        })
        
    except Exception as e:
        logger.logger.error(f"Error in background playlist download: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
        
        # Send an error event
        fire_event("playlist_download_error", {
            "playlist_id": playlist_id,
            "error": str(e)
        })
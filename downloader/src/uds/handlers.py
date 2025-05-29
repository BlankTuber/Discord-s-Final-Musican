import ytdlp_handler
import json
import time
import traceback
from uds import protocol

_config = {}
_command_handlers = {}
_event_listeners = []

def init(cfg):
    global _config
    _config.update(cfg)
    
    register_default_handlers()
    
    ytdlp_handler.register_event_callback(handle_ytdlp_event)
    
    print("UDS handlers module initialized")

def register_handler(command, handler_func):
    if not callable(handler_func):
        return False
        
    _command_handlers[command] = handler_func
    return True

def register_event_listener(listener_func):
    global _event_listeners
    if callable(listener_func) and listener_func not in _event_listeners:
        _event_listeners.append(listener_func)
        return True
    return False

def handle_ytdlp_event(event_type, event_data):
    for listener in _event_listeners:
        try:
            listener(event_type, event_data)
        except Exception as e:
            print(f"Error in event listener: {e}")
            print(traceback.format_exc())

def register_default_handlers():
    register_handler("download_audio", handle_download_audio)
    register_handler("download_playlist", handle_download_playlist)
    register_handler("download_playlist_item", handle_download_playlist_item)
    register_handler("get_playlist_info", handle_get_playlist_info)
    register_handler("start_playlist_download", handle_start_playlist_download)
    register_handler("get_playlist_download_status", handle_get_playlist_download_status)
    register_handler("search", handle_search)
    register_handler("ping", handle_ping)

def process_request(request, config):
    command = request.get("command")
    request_id = request.get("id")
    params = request.get("params", {})
    
    start_time = time.time()
    
    if command == "ping" and params.get("keepalive"):
        print(f"UDS: Received keepalive ping - ID: {request_id}")
    else:
        print(f"UDS: Received request - Command: {command}, ID: {request_id}")
    
    handler = _command_handlers.get(command)
    
    if not handler:
        print(f"UDS: Unknown command: {command}")
        return protocol.create_error_response(
            f"Unknown command: {command}", 
            request_id
        )
    
    try:
        if command == "ping" and params.get("keepalive"):
            print(f"UDS: Processing keepalive ping")
        else:
            print(f"UDS: Processing {command} with params: {json.dumps(params, default=str)}")
        
        result = handler(params, config)
        elapsed = time.time() - start_time
        
        if command == "ping" and params.get("keepalive"):
            print(f"UDS: Keepalive ping processed successfully in {elapsed:.3f} seconds")
        else:
            print(f"UDS: {command} processed successfully in {elapsed:.2f} seconds")
        
        return protocol.create_success_response(request_id, result)
    except Exception as e:
        elapsed = time.time() - start_time
        print(f"UDS: Error processing {command} after {elapsed:.2f} seconds: {str(e)}")
        print(f"UDS: Traceback: {traceback.format_exc()}")
        return protocol.create_error_response(
            f"Error processing {command}: {str(e)}", 
            request_id
        )

def handle_download_audio(params, config):
    url = params.get("url")
    
    if not url:
        print("UDS: URL is required for download_audio")
        raise ValueError("URL is required")
    
    max_duration = params.get("max_duration_seconds")
    max_size = params.get("max_size_mb")
    allow_live = params.get("allow_live", False)
    
    print(f"UDS: Downloading audio from URL: {url}")
    result = ytdlp_handler.download_audio(
        url, 
        max_duration_seconds=max_duration, 
        max_size_mb=max_size, 
        allow_live=allow_live
    )
    
    if not result:
        print(f"UDS: Download failed for URL: {url}")
        raise Exception("Download failed")
    
    print(f"UDS: Download completed for URL: {url}")
    if "title" in result:
        print(f"UDS: Downloaded: {result['title']}")
        
    return result

def handle_download_playlist(params, config):
    url = params.get("url")
    
    if not url:
        print("UDS: URL is required for download_playlist")
        raise ValueError("URL is required")
    
    max_items = params.get("max_items")
    max_duration = params.get("max_duration_seconds")
    max_size = params.get("max_size_mb")
    allow_live = params.get("allow_live", False)
    requester = params.get("requester")
    guild_id = params.get("guild_id")
    
    print(f"UDS: Downloading playlist from URL: {url}, max items: {max_items}")
    start_time = time.time()
    
    result = ytdlp_handler.download_playlist(
        url, 
        max_items=max_items,
        max_duration_seconds=max_duration, 
        max_size_mb=max_size, 
        allow_live=allow_live,
        requester=requester,
        guild_id=guild_id
    )
    
    elapsed = time.time() - start_time
    
    if not result:
        print(f"UDS: Playlist download failed for URL: {url} after {elapsed:.2f} seconds")
        raise Exception("Playlist download failed")
    
    item_count = result.get("count", 0)
    successful = result.get("successful_downloads", 0)
    print(f"UDS: Playlist download completed in {elapsed:.2f} seconds, {successful} of {item_count} tracks downloaded")
        
    return result

def handle_start_playlist_download(params, config):
    url = params.get("url")
    
    if not url:
        print("UDS: URL is required for start_playlist_download")
        raise ValueError("URL is required")
    
    max_items = params.get("max_items")
    max_duration = params.get("max_duration_seconds")
    max_size = params.get("max_size_mb")
    allow_live = params.get("allow_live", False)
    requester = params.get("requester")
    guild_id = params.get("guild_id")
    
    print(f"UDS: Starting async playlist download from URL: {url}, max items: {max_items}")
    
    result = ytdlp_handler.start_playlist_download(
        url, 
        max_items=max_items,
        max_duration_seconds=max_duration, 
        max_size_mb=max_size, 
        allow_live=allow_live,
        requester=requester,
        guild_id=guild_id
    )
    
    if not result or result.get("status") == "error":
        print(f"UDS: Starting playlist download failed for URL: {url}")
        raise Exception(result.get("message", "Starting playlist download failed"))
    
    print(f"UDS: Started playlist download for '{result.get('playlist_title')}' with {result.get('total_tracks')} tracks")
    
    return result

def handle_get_playlist_download_status(params, config):
    playlist_id = params.get("playlist_id")
    
    if not playlist_id:
        print("UDS: playlist_id is required for get_playlist_download_status")
        raise ValueError("playlist_id is required")
    
    return {
        "playlist_id": playlist_id,
        "status": "in_progress",
        "total_tracks": 0,
        "downloaded_tracks": 0,
        "failed_tracks": 0
    }

def handle_get_playlist_info(params, config):
    url = params.get("url")
    
    if not url:
        print("UDS: URL is required for get_playlist_info")
        raise ValueError("URL is required")
    
    max_items = params.get("max_items")
    
    print(f"UDS: Getting playlist info from URL: {url}, max items: {max_items}")
    start_time = time.time()
    
    result = ytdlp_handler.get_playlist_info(
        url, 
        max_items=max_items
    )
    
    elapsed = time.time() - start_time
    
    if not result:
        print(f"UDS: Getting playlist info failed for URL: {url} after {elapsed:.2f} seconds")
        raise Exception("Getting playlist info failed")
    
    item_count = result.get("total_tracks", 0)
    print(f"UDS: Playlist info retrieved in {elapsed:.2f} seconds, found {item_count} tracks")
        
    return result

def handle_download_playlist_item(params, config):
    url = params.get("url")
    
    if not url:
        print("UDS: URL is required for download_playlist_item")
        raise ValueError("URL is required")
    
    index = params.get("index")
    if index is None:
        print("UDS: Index is required for download_playlist_item")
        raise ValueError("Index is required")
    
    max_duration = params.get("max_duration_seconds")
    max_size = params.get("max_size_mb")
    allow_live = params.get("allow_live", False)
    
    print(f"UDS: Downloading playlist item {index} from URL: {url}")
    start_time = time.time()
    
    result = ytdlp_handler.download_playlist_item(
        url, 
        index,
        max_duration_seconds=max_duration, 
        max_size_mb=max_size, 
        allow_live=allow_live
    )
    
    elapsed = time.time() - start_time
    
    if not result:
        print(f"UDS: Playlist item download failed for URL: {url}, index: {index} after {elapsed:.2f} seconds")
        raise Exception(f"Playlist item download failed for index {index}")
    
    print(f"UDS: Playlist item download completed in {elapsed:.2f} seconds")
    if "title" in result:
        print(f"UDS: Downloaded: {result['title']}")
        
    return result

def handle_search(params, config):
    query = params.get("query")
    
    if not query:
        print("UDS: Search query is required")
        raise ValueError("Search query is required")
    
    platform = params.get("platform", "youtube")
    limit = params.get("limit", 5)
    include_live = params.get("include_live", False)
    
    print(f"UDS: Searching for '{query}' on {platform}, limit: {limit}")
    start_time = time.time()
    
    results = ytdlp_handler.search(
        query, 
        platform=platform, 
        limit=limit, 
        include_live=include_live
    )
    
    elapsed = time.time() - start_time
    
    if not results:
        print(f"UDS: No search results found after {elapsed:.2f} seconds")
        return {"results": []}
    
    result_count = 0
    if "results" in results and isinstance(results["results"], list):
        result_count = len(results["results"])
    
    print(f"UDS: Search completed in {elapsed:.2f} seconds, found {result_count} results")
        
    return results

def handle_ping(params, config):
    is_keepalive = params.get("keepalive", False)
    timestamp = params.get("timestamp", "none")
    
    if is_keepalive:
        print("UDS: Received keepalive ping request")
    else:
        print("UDS: Received ping request")
    
    response = {
        "message": "pong",
        "timestamp": timestamp,
        "server_time": time.time()
    }
    
    if is_keepalive:
        response["keepalive"] = True
    
    return response
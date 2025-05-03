import ytdlp_handler
import json
import time
import traceback
from uds import protocol

_config = {}
_command_handlers = {}

def init(cfg):
    global _config
    _config.update(cfg)
    
    register_default_handlers()
    
    print("UDS handlers module initialized")

def register_handler(command, handler_func):
    if not callable(handler_func):
        return False
        
    _command_handlers[command] = handler_func
    return True

def register_default_handlers():
    register_handler("download_audio", handle_download_audio)
    register_handler("download_playlist", handle_download_playlist)
    register_handler("search", handle_search)
    register_handler("ping", handle_ping)

def process_request(request, config):
    command = request.get("command")
    request_id = request.get("id")
    params = request.get("params", {})
    
    start_time = time.time()
    print(f"UDS: Received request - Command: {command}, ID: {request_id}")
    
    handler = _command_handlers.get(command)
    
    if not handler:
        print(f"UDS: Unknown command: {command}")
        return protocol.create_error_response(
            f"Unknown command: {command}", 
            request_id
        )
    
    try:
        print(f"UDS: Processing {command} with params: {json.dumps(params, default=str)}")
        result = handler(params, config)
        elapsed = time.time() - start_time
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
    
    print(f"UDS: Downloading playlist from URL: {url}, max items: {max_items}")
    start_time = time.time()
    
    result = ytdlp_handler.download_playlist(
        url, 
        max_items=max_items,
        max_duration_seconds=max_duration, 
        max_size_mb=max_size, 
        allow_live=allow_live
    )
    
    elapsed = time.time() - start_time
    
    if not result:
        print(f"UDS: Playlist download failed for URL: {url} after {elapsed:.2f} seconds")
        raise Exception("Playlist download failed")
    
    item_count = result.get("count", 0)
    successful = result.get("successful_downloads", 0)
    print(f"UDS: Playlist download completed in {elapsed:.2f} seconds, {successful} of {item_count} tracks downloaded")
        
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
    print("UDS: Received ping request")
    return {
        "message": "pong",
        "timestamp": params.get("timestamp", "none")
    }
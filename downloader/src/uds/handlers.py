import ytdlp_handler
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
    
    handler = _command_handlers.get(command)
    
    if not handler:
        return protocol.create_error_response(
            f"Unknown command: {command}", 
            request_id
        )
    
    try:
        result = handler(params, config)
        return protocol.create_success_response(request_id, result)
    except Exception as e:
        return protocol.create_error_response(
            f"Error processing {command}: {str(e)}", 
            request_id
        )

def handle_download_audio(params, config):
    url = params.get("url")
    
    if not url:
        raise ValueError("URL is required")
    
    max_duration = params.get("max_duration_seconds")
    max_size = params.get("max_size_mb")
    allow_live = params.get("allow_live", False)
    
    result = ytdlp_handler.download_audio(
        url, 
        max_duration_seconds=max_duration, 
        max_size_mb=max_size, 
        allow_live=allow_live
    )
    
    if not result:
        raise Exception("Download failed")
        
    return result

def handle_download_playlist(params, config):
    url = params.get("url")
    
    if not url:
        raise ValueError("URL is required")
    
    max_items = params.get("max_items")
    max_duration = params.get("max_duration_seconds")
    max_size = params.get("max_size_mb")
    allow_live = params.get("allow_live", False)
    
    result = ytdlp_handler.download_playlist(
        url, 
        max_items=max_items,
        max_duration_seconds=max_duration, 
        max_size_mb=max_size, 
        allow_live=allow_live
    )
    
    if not result:
        raise Exception("Playlist download failed")
        
    return result

def handle_search(params, config):
    query = params.get("query")
    
    if not query:
        raise ValueError("Search query is required")
    
    platform = params.get("platform", "youtube")
    limit = params.get("limit", 5)
    include_live = params.get("include_live", False)
    
    results = ytdlp_handler.search(
        query, 
        platform=platform, 
        limit=limit, 
        include_live=include_live
    )
    
    if not results:
        return {"results": []}
        
    return results

def handle_ping(params, config):
    return {
        "message": "pong",
        "timestamp": params.get("timestamp", "none")
    }
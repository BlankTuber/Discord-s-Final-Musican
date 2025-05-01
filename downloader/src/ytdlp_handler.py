from ytdlp import audio, playlist, search as search_module, utils

config = {}

def initialize(cfg):
    global config
    config.update(cfg)
    utils.init(config)

def download_audio(url, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return None
    
    return audio.download(
        url, 
        config["download_path"], 
        max_duration_seconds=max_duration_seconds, 
        max_size_mb=max_size_mb, 
        allow_live=allow_live
    )

def download_playlist(url, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    if platform not in config["allowed_origins"]:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return None
    
    return playlist.download(
        url, 
        config["download_path"], 
        max_items=max_items,
        max_duration_seconds=max_duration_seconds, 
        max_size_mb=max_size_mb, 
        allow_live=allow_live
    )

def search(query, platform='youtube', limit=5, include_live=False):
    # Normalize platform string
    platform_lower = platform.lower()
    allowed_platform = None
    
    # Map user input to specific platform identifiers
    if platform_lower in ['youtube', 'youtu.be', 'youtube.com', 'https://youtube.com', 'https://youtu.be']:
        allowed_platform = 'https://youtube.com'
    elif platform_lower in ['soundcloud', 'soundcloud.com', 'https://soundcloud.com']:
        allowed_platform = 'https://soundcloud.com'
    elif platform_lower in ['music.youtube.com', 'ytmusic', 'youtube music', 'https://music.youtube.com']:
        allowed_platform = 'https://music.youtube.com'
    else:
        # Try to get the platform from the URL
        allowed_platform = utils.get_platform(platform)
    
    if allowed_platform not in config["allowed_origins"]:
        print(f"Platform '{allowed_platform}' is not in the allowed origins list.")
        print(f"Allowed origins: {config['allowed_origins']}")
        return None
    
    results = search_module.find(
        query, 
        platform=platform, 
        limit=limit, 
        include_live=include_live
    )
    
    if not results:
        return {"results": []}
        
    return {"results": results}
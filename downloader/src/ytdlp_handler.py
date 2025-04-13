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
    if platform == 'youtube' or platform == 'https://youtube.com':
        allowed_platform = 'https://youtube.com'
    elif platform == 'soundcloud' or platform == 'https://soundcloud.com':
        allowed_platform = 'https://soundcloud.com'
    else:
        print(f"Search not supported for platform: {platform}")
        return None
    
    if allowed_platform not in config["allowed_origins"]:
        print(f"Platform '{allowed_platform}' is not in the allowed origins list.")
        return None
    
    return search_module.find(
        query, 
        platform=platform, 
        limit=limit, 
        include_live=include_live
    )
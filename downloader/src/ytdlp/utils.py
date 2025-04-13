import os
import re

config = {}

def init(cfg):
    global config
    config.update(cfg)

def get_platform(url):
    url = url.lower()
    
    if 'youtube.com' in url or 'youtu.be' in url:
        return 'https://youtube.com'
    elif 'music.youtube.com' in url:
        return 'https://music.youtube.com'  
    elif 'soundcloud.com' in url:
        return 'https://soundcloud.com'
    elif 'spotify.com' in url:
        return 'https://spotify.com'
    elif 'bandcamp.com' in url:
        return 'https://bandcamp.com'
    
    match = re.search(r'https?://([^/]+)', url)
    if match:
        domain = match.group(1)
        if domain.startswith('www.'):
            domain = domain[4:]
        return f"https://{domain}"
    
    return "unknown"

def get_platform_prefix(platform):
    if 'youtube.com' in platform:
        return 'youtube'
    elif 'music.youtube.com' in platform:
        return 'ytmusic'
    elif 'soundcloud.com' in platform:
        return 'soundcloud'
    elif 'spotify.com' in platform:
        return 'spotify'
    elif 'bandcamp.com' in platform:
        return 'bandcamp'
    
    match = re.search(r'https?://([^/\.]+)', platform)
    if match:
        return match.group(1)
    
    return "unknown"

def sanitize_string(text):
    if not text:
        return "untitled"
    
    text = re.sub(r'[\\/*?:"<>|]', '', text)
    text = re.sub(r'[\s\-\+]+', '_', text)
    text = re.sub(r'[^\w\-\.]', '', text)
    if len(text) > 100:
        text = text[:100]
    
    return text

def ensure_directory(directory_path):
    os.makedirs(directory_path, exist_ok=True)
    return directory_path

def match_filter_func(info, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    if not allow_live and info.get('duration') is None:
        return "Video is a live stream (duration is None)"
        
    if max_duration_seconds and info.get('duration', 0) > max_duration_seconds:
        return f"The video is too long ({info['duration']} seconds > {max_duration_seconds} seconds)"
    
    if max_size_mb:
        max_bytes = max_size_mb * 1024 * 1024
        if info.get('filesize_approx', 0) > max_bytes:
            return f"The file is too large ({info['filesize_approx'] / (1024*1024):.1f}MB > {max_size_mb}MB)"
    
    if not allow_live and info.get('duration', 0) > 12 * 3600:
        title = info.get('title', '').lower()
        if any(keyword in title for keyword in ['live', 'radio', '24/7', 'stream']):
            return "Video appears to be a live stream (extremely long duration with stream keywords in title)"
    
    return None

def progress_hook(d):
    if d['status'] == 'downloading':
        percent = d.get('_percent_str', 'N/A')
        speed = d.get('_speed_str', 'N/A')
        print(f"Downloading: {percent} at {speed}")
    elif d['status'] == 'finished':
        print("Download complete! Converting to MP3...")
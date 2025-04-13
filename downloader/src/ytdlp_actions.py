import yt_dlp
import os
import config
import re

def download_audio(url, max_duration_seconds=None, max_size_mb=None):
    platform = get_platform(url)
    
    if platform not in config.ALLOWED_ORIGINS:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        return None
    
    ydl_opts = {
        'format': 'bestaudio/best',
        'postprocessors': [{
            'key': 'FFmpegExtractAudio',
            'preferredcodec': 'mp3',
            'preferredquality': '192',
        }],
        'outtmpl': os.path.join(config.DOWNLOAD_PATH, '%(title)s.%(ext)s'),
        'progress_hooks': [progress_hook],
        'match_filter': build_match_filter(max_duration_seconds, max_size_mb),
        'ignoreerrors': False,
    }
    
    try:
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            info = ydl.extract_info(url, download=False)
            
            if max_duration_seconds and info.get('duration', 0) > max_duration_seconds:
                print(f"Skipping: Duration ({info.get('duration')}s) exceeds limit ({max_duration_seconds}s)")
                return None
                
            if max_size_mb and info.get('filesize_approx', 0) > max_size_mb * 1024 * 1024:
                print(f"Skipping: Estimated size ({info.get('filesize_approx') / (1024*1024):.1f}MB) exceeds limit ({max_size_mb}MB)")
                return None
            
            info = ydl.extract_info(url, download=True)
            
            filename = ydl.prepare_filename(info).replace(os.path.splitext(ydl.prepare_filename(info))[1], '.mp3')
            return {
                'title': info.get('title', 'Unknown'),
                'filename': filename,
                'duration': info.get('duration'),
                'file_size': info.get('filesize'),
                'platform': platform
            }
            
    except yt_dlp.utils.DownloadError as e:
        if "premium" in str(e).lower() or "paywall" in str(e).lower() or "login" in str(e).lower():
            print(f"Download error: Content is behind a paywall or requires login")
        else:
            print(f"Download error: {e}")
        return None
    except Exception as e:
        print(f"Error downloading audio: {e}")
        return None

def download_playlist(url, max_items=None, max_duration_seconds=None, max_size_mb=None):
    platform = get_platform(url)
    
    if platform not in config.ALLOWED_ORIGINS:
        print(f"Platform '{platform}' is not in the allowed origins list.")
        return None
    
    ydl_opts = {
        'format': 'bestaudio/best',
        'postprocessors': [{
            'key': 'FFmpegExtractAudio',
            'preferredcodec': 'mp3',
            'preferredquality': '192',
        }],
        'outtmpl': os.path.join(config.DOWNLOAD_PATH, '%(playlist)s/%(playlist_index)s - %(title)s.%(ext)s'),
        'progress_hooks': [progress_hook],
        'match_filter': build_match_filter(max_duration_seconds, max_size_mb),
        'ignoreerrors': True,
        'noplaylist': False,
    }
    
    if max_items:
        ydl_opts['playlistend'] = max_items
    
    try:
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            info = ydl.extract_info(url, download=True)
            
            if not info.get('entries'):
                print("No items found in playlist or not a playlist URL")
                return None
            
            results = []
            for entry in info.get('entries', []):
                if entry:
                    filename = ydl.prepare_filename(entry).replace(os.path.splitext(ydl.prepare_filename(entry))[1], '.mp3')
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'filename': filename,
                        'duration': entry.get('duration'),
                        'file_size': entry.get('filesize'),
                        'platform': platform
                    })
            
            return {
                'playlist_title': info.get('title', 'Unknown Playlist'),
                'playlist_url': url,
                'count': len(results),
                'items': results
            }
            
    except yt_dlp.utils.DownloadError as e:
        if "premium" in str(e).lower() or "paywall" in str(e).lower() or "login" in str(e).lower():
            print(f"Download error: Content is behind a paywall or requires login")
        else:
            print(f"Download error: {e}")
        return None
    except Exception as e:
        print(f"Error downloading playlist: {e}")
        return None

def search(query, platform='youtube', limit=5):
    search_url = None
    
    if platform == 'youtube' or platform == 'https://youtube.com':
        search_url = f'ytsearch{limit}:{query}'
        allowed_platform = 'https://youtube.com'
    elif platform == 'soundcloud' or platform == 'https://soundcloud.com':
        search_url = f'scsearch{limit}:{query}'
        allowed_platform = 'https://soundcloud.com'
    else:
        print(f"Search not supported for platform: {platform}")
        return None
    
    if allowed_platform not in config.ALLOWED_ORIGINS:
        print(f"Platform '{allowed_platform}' is not in the allowed origins list.")
        return None
    
    ydl_opts = {
        'format': 'bestaudio/best',
        'skip_download': True,
        'quiet': True,
        'no_warnings': True,
        'ignoreerrors': True
    }
    
    try:
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            info = ydl.extract_info(search_url, download=False)
            
            if not info.get('entries'):
                print(f"No results found for query: {query}")
                return None
            
            results = []
            for entry in info.get('entries', []):
                if entry:
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'url': entry.get('webpage_url'),
                        'duration': entry.get('duration'),
                        'uploader': entry.get('uploader'),
                        'thumbnail': entry.get('thumbnail'),
                        'platform': allowed_platform
                    })
            
            return results
            
    except Exception as e:
        print(f"Error searching: {e}")
        return None

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

def build_match_filter(max_duration_seconds, max_size_mb):
    filters = []
    
    if max_duration_seconds:
        filters.append(f"duration <= {max_duration_seconds}")
    
    if max_size_mb:
        max_bytes = max_size_mb * 1024 * 1024
        filters.append(f"filesize_approx <= {max_bytes}")
    
    if filters:
        return " and ".join(filters)
    else:
        return None

def progress_hook(d):
    if d['status'] == 'downloading':
        percent = d.get('_percent_str', 'N/A')
        speed = d.get('_speed_str', 'N/A')
        print(f"Downloading: {percent} at {speed}")
    elif d['status'] == 'finished':
        print("Download complete! Converting to MP3...")
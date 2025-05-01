import yt_dlp
from ytdlp import utils

def find(query, platform='youtube', limit=5, include_live=False):
    search_url = None
    allowed_platform = None
    
    platform = platform.lower()
    
    # Handle different platform inputs
    if platform in ['youtube', 'youtu.be', 'youtube.com', 'https://youtube.com', 'https://youtu.be']:
        search_url = f'ytsearch{limit*2}:{query}'
        allowed_platform = 'https://youtube.com'
    elif platform in ['soundcloud', 'soundcloud.com', 'https://soundcloud.com']:
        search_url = f'scsearch{limit*2}:{query}'
        allowed_platform = 'https://soundcloud.com'
    elif platform in ['music.youtube.com', 'ytmusic', 'youtube music', 'https://music.youtube.com']:
        search_url = f'ytsearch{limit*2}:{query} site:music.youtube.com'
        allowed_platform = 'https://music.youtube.com'
    else:
        print(f"Search not supported for platform: {platform}")
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
            
            if not info or not info.get('entries'):
                print(f"No results found for query: {query}")
                return None
            
            results = []
            for entry in info.get('entries', []):
                if not entry:
                    continue
                    
                if not include_live and entry.get('duration') is None:
                    print(f"Skipping live stream: {entry.get('title', 'Unknown')}")
                    continue
                
                if not include_live and entry.get('duration') is None and 'live' in entry.get('title', '').lower():
                    print(f"Skipping likely live stream: {entry.get('title', 'Unknown')}")
                    continue
                    
                if not include_live and ('radio' in entry.get('title', '').lower() or '24/7' in entry.get('title', '')):
                    if entry.get('duration') is None or entry.get('duration', 0) > 12 * 3600:
                        print(f"Skipping likely radio stream: {entry.get('title', 'Unknown')}")
                        continue
                
                # Extract thumbnail URL properly
                thumbnail = entry.get('thumbnail', '')
                if isinstance(thumbnail, dict) and 'url' in thumbnail:
                    thumbnail = thumbnail['url']
                
                # Get direct URL for the video/audio
                url = entry.get('webpage_url') or entry.get('url', '')
                
                # Get uploader information
                uploader = entry.get('uploader', entry.get('channel', 'Unknown'))
                
                results.append({
                    'title': entry.get('title', 'Unknown'),
                    'url': url,
                    'duration': entry.get('duration'),
                    'uploader': uploader,
                    'thumbnail': thumbnail,
                    'platform': allowed_platform,
                    'id': entry.get('id', ''),
                    'live_status': entry.get('is_live', False)
                })
                
                if len(results) >= limit:
                    break
            
            return results
            
    except Exception as e:
        print(f"Error searching: {e}")
        return None
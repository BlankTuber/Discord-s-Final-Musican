import yt_dlp
from ytdlp import utils

def find(query, platform='youtube', limit=5, include_live=False):
    search_url = None
    
    if platform == 'youtube' or platform == 'https://youtube.com':
        search_url = f'ytsearch{limit*2}:{query}'
        allowed_platform = 'https://youtube.com'
    elif platform == 'soundcloud' or platform == 'https://soundcloud.com':
        search_url = f'scsearch{limit*2}:{query}'
        allowed_platform = 'https://soundcloud.com'
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
                
                results.append({
                    'title': entry.get('title', 'Unknown'),
                    'url': entry.get('webpage_url'),
                    'duration': entry.get('duration'),
                    'uploader': entry.get('uploader'),
                    'thumbnail': entry.get('thumbnail'),
                    'platform': allowed_platform
                })
                
                if len(results) >= limit:
                    break
            
            return results
            
    except Exception as e:
        print(f"Error searching: {e}")
        return None
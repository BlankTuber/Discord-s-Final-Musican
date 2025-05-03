import yt_dlp
import time
import traceback
from ytdlp import utils

def find(query, platform='youtube', limit=5, include_live=False):
    print(f"SEARCH: Starting search for '{query}' on platform '{platform}', limit: {limit}")
    start_time = time.time()
    
    search_url = None
    allowed_platform = None
    
    platform = platform.lower()
    
    # Set longer timeout for YouTube Music searches
    timeout = 60  # Default timeout
    if platform in ['music.youtube.com', 'ytmusic', 'youtube music', 'https://music.youtube.com']:
        timeout = 120  # Extended timeout for YouTube Music
    
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
        print(f"SEARCH: Search not supported for platform: {platform}")
        return None
    
    print(f"SEARCH: Using search URL: {search_url}")
    
    ydl_opts = {
        'format': 'bestaudio/best',
        'skip_download': True,
        'quiet': True,
        'no_warnings': True,
        'ignoreerrors': True,
        'socket_timeout': timeout  # Use dynamic timeout
    }
    
    try:
        print("SEARCH: Starting yt-dlp extraction")
        ytdlp_start = time.time()
        
        with yt_dlp.YoutubeDL(ydl_opts) as ydl:
            print("SEARCH: Calling extract_info...")
            info = ydl.extract_info(search_url, download=False)
            
            ytdlp_elapsed = time.time() - ytdlp_start
            print(f"SEARCH: yt-dlp extraction completed in {ytdlp_elapsed:.2f} seconds")
            
            if not info:
                print("SEARCH: No info returned from extract_info")
                return None
                
            if not info.get('entries'):
                print(f"SEARCH: No results found for query: {query}")
                return None
            
            entries = info.get('entries', [])
            print(f"SEARCH: Got {len(entries)} initial entries")
            
            results = []
            processed = 0
            skipped_live = 0
            
            for entry in entries:
                if not entry:
                    print("SEARCH: Skipping None entry")
                    continue
                
                processed += 1
                    
                is_live = entry.get('duration') is None
                title = entry.get('title', 'Unknown')
                
                if not include_live and is_live:
                    print(f"SEARCH: Skipping live stream: {title}")
                    skipped_live += 1
                    continue
                
                if not include_live and is_live and 'live' in title.lower():
                    print(f"SEARCH: Skipping likely live stream by title: {title}")
                    skipped_live += 1
                    continue
                    
                if not include_live and ('radio' in title.lower() or '24/7' in title.lower()):
                    if is_live or entry.get('duration', 0) > 12 * 3600:
                        print(f"SEARCH: Skipping likely radio stream: {title}")
                        skipped_live += 1
                        continue
                
                # Extract thumbnail URL properly
                thumbnail = entry.get('thumbnail', '')
                if isinstance(thumbnail, dict) and 'url' in thumbnail:
                    thumbnail = thumbnail['url']
                
                # Get direct URL for the video/audio
                url = entry.get('webpage_url') or entry.get('url', '')
                
                # Get uploader information
                uploader = entry.get('uploader', entry.get('channel', 'Unknown'))
                
                print(f"SEARCH: Adding result {len(results)+1}: {title}")
                
                results.append({
                    'title': title,
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
            
            elapsed = time.time() - start_time
            print(f"SEARCH: Finished processing in {elapsed:.2f} seconds")
            print(f"SEARCH: Processed {processed} entries, skipped {skipped_live} live streams, returning {len(results)} results")
            
            return results
            
    except Exception as e:
        elapsed = time.time() - start_time
        print(f"SEARCH: Error after {elapsed:.2f} seconds: {e}")
        print(f"SEARCH: Traceback: {traceback.format_exc()}")
        return None
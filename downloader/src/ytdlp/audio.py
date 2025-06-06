import os
import time
import yt_dlp
from ytdlp import utils

def match_filter_func(info, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    if not allow_live and info.get('duration') is None:
        return "Video is a live stream (duration is None)"
        
    if max_duration_seconds is not None and info.get('duration', 0) > max_duration_seconds:
        return f"The video is too long ({info['duration']} seconds > {max_duration_seconds} seconds)"
    
    if max_size_mb is not None:
        max_bytes = max_size_mb * 1024 * 1024
        if info.get('filesize_approx', 0) > max_bytes:
            return f"The file is too large ({info['filesize_approx'] / (1024*1024):.1f}MB > {max_size_mb}MB)"
    
    if not allow_live and info.get('duration', 0) > 12 * 3600:
        title = info.get('title', '').lower()
        if any(keyword in title for keyword in ['live', 'radio', '24/7', 'stream']):
            return "Video appears to be a live stream (extremely long duration with stream keywords in title)"
    
    return None

def download(url, download_path, db, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    platform_prefix = utils.get_platform_prefix(platform)
    
    try:
        song = db.get_song_by_url(url)
        if song and os.path.exists(song['file_path']):
            print(f"Song already exists in database and file exists: {song['title']}")
            return {
                'id': song['id'],
                'title': song['title'],
                'filename': song['file_path'],
                'duration': song['duration'],
                'file_size': song['file_size'],
                'platform': song['platform'],
                'artist': song['artist'] if 'artist' in song else '',
                'thumbnail_url': song['thumbnail_url'] if 'thumbnail_url' in song else '',
                'is_stream': bool(song['is_stream']) if 'is_stream' in song else False,
                'skipped': True
            }
            
        with yt_dlp.YoutubeDL({
            'skip_download': True, 
            'quiet': True,
            'socket_timeout': 15
        }) as ydl:
            info = ydl.extract_info(url, download=False)
            
            if not info:
                print(f"No info found for URL: {url}")
                return None
            
            if not allow_live and info.get('duration') is None:
                error_msg = "Content is a live stream (duration is None)"
                print(f"Skipping: {error_msg}")
                return {'status': 'error', 'message': error_msg}
                
            if max_duration_seconds is not None and info.get('duration', 0) > max_duration_seconds:
                error_msg = f"Duration ({info.get('duration')}s) exceeds limit ({max_duration_seconds}s)"
                print(f"Skipping: {error_msg}")
                return {'status': 'error', 'message': error_msg}
                
            if max_size_mb is not None and info.get('filesize_approx', 0) > max_size_mb * 1024 * 1024:
                error_msg = f"Estimated size ({info.get('filesize_approx') / (1024*1024):.1f}MB) exceeds limit ({max_size_mb}MB)"
                print(f"Skipping: {error_msg}")
                return {'status': 'error', 'message': error_msg}
            
            filename = f"{platform_prefix}_{info['id']}.mp3"
            full_path = os.path.abspath(os.path.join(download_path, filename))
            
            if os.path.isfile(full_path):
                print(f"File exists but not in database: {full_path}")
                file_exists = True
                file_size = os.path.getsize(full_path)
            else:
                file_exists = False
                file_size = None
        
        song_count = db.get_song_count()
        if song_count >= 500:
            print(f"Warning: Database contains {song_count} songs, which is at or above the limit of 500.")
            print("The janitor will clean up old songs on its next run.")
        
        if file_exists:
            print(f"File already exists, skipping download: {full_path}")
        else:
            ydl_opts = {
                'format': 'bestaudio/best',
                'postprocessors': [{
                    'key': 'FFmpegExtractAudio',
                    'preferredcodec': 'mp3',
                    'preferredquality': '192',
                }],
                'outtmpl': os.path.join(download_path, f"{platform_prefix}_%(id)s.%(ext)s"),
                'progress_hooks': [utils.progress_hook],
                'ignoreerrors': False,
                'nooverwrites': True,
                'socket_timeout': 30,
                'retries': 3,
                'fragment_retries': 3,
                'extractor_retries': 3
            }
            
            if max_duration_seconds is not None or max_size_mb is not None:
                ydl_opts['match_filter'] = lambda info: utils.match_filter_func(
                    info, max_duration_seconds, max_size_mb, allow_live
                )
            
            try:
                with yt_dlp.YoutubeDL(ydl_opts) as ydl:
                    info = ydl.extract_info(url, download=True)
                    
                    if not info:
                        error_msg = "Failed to extract info during download"
                        print(f"{error_msg} for: {url}")
                        return {'status': 'error', 'message': error_msg}
                    
                    filename = f"{platform_prefix}_{info['id']}.mp3"
                    full_path = os.path.join(download_path, filename)
                    
                    file_exists = os.path.exists(full_path)
                    if not file_exists:
                        error_msg = "Download completed but file not found"
                        print(f"{error_msg}: {full_path}")
                        return {'status': 'error', 'message': error_msg}
                    
                    file_size = os.path.getsize(full_path)
            except yt_dlp.utils.DownloadError as e:
                error_msg = str(e)
                print(f"Download error: {error_msg}")
                return {'status': 'error', 'message': error_msg}
        
        if file_exists:
            with yt_dlp.YoutubeDL({
                'skip_download': True, 
                'quiet': True
            }) as ydl:
                info = ydl.extract_info(url, download=False)
                
                if not info:
                    print(f"Failed to extract info for database entry: {url}")
                    info = {'title': 'Unknown', 'id': os.path.basename(full_path).split('.')[0]}
            
            thumbnail = info.get('thumbnail', '')
            if isinstance(thumbnail, dict) and 'url' in thumbnail:
                thumbnail = thumbnail['url']
            
            artist = info.get('artist', info.get('uploader', info.get('channel', 'Unknown')))
            
            existing_song = db.get_song_by_url(url)
            if existing_song:
                print(f"Song already exists in database: {existing_song['title']}")
                song_id = existing_song['id']
            else:
                song_id = db.add_song(
                    title=info.get('title', 'Unknown'),
                    url=url,
                    platform=platform,
                    file_path=full_path,
                    duration=info.get('duration'),
                    file_size=file_size,
                    thumbnail_url=thumbnail,
                    artist=artist,
                    is_stream=info.get('is_live', False)
                )
                print(f"Added song to database with ID: {song_id}")
            
            time.sleep(0.2)
            
            return {
                'id': song_id,
                'title': info.get('title', 'Unknown'),
                'filename': full_path,
                'duration': info.get('duration'),
                'file_size': file_size,
                'platform': platform,
                'artist': artist,
                'thumbnail_url': thumbnail,
                'is_stream': info.get('is_live', False),
                'skipped': False
            }
        else:
            error_msg = f"File does not exist after download"
            print(f"{error_msg}: {full_path}")
            return {'status': 'error', 'message': error_msg}
            
    except yt_dlp.utils.DownloadError as e:
        error_msg = str(e).lower()
        
        if "private" in error_msg:
            print(f"Download error: This video is private")
            return {'status': 'error', 'message': "This video is private"}
        elif any(term in error_msg for term in ["premium", "paywall", "subscribe", "login", "member", "paid"]):
            print(f"Download error: This content requires a premium account or login")
            return {'status': 'error', 'message': "This content requires a premium account or login"}
        elif any(term in error_msg for term in ["removed", "deleted", "taken down"]):
            print(f"Download error: This video has been removed or deleted")
            return {'status': 'error', 'message': "This video has been removed or deleted"}
        elif "unavailable" in error_msg:
            print(f"Download error: This video is unavailable")
            return {'status': 'error', 'message': "This video is unavailable"}
        elif "copyright" in error_msg:
            print(f"Download error: This video is blocked due to copyright issues")
            return {'status': 'error', 'message': "This video is blocked due to copyright issues"}
        elif "age" in error_msg and ("restrict" in error_msg or "verify" in error_msg):
            print(f"Download error: This video is age-restricted")
            return {'status': 'error', 'message': "This video is age-restricted"}
        elif ("geo" in error_msg and "block" in error_msg) or "country" in error_msg:
            print(f"Download error: This video is not available in your country")
            return {'status': 'error', 'message': "This video is not available in your country"}
        elif "not exist" in error_msg or "no longer" in error_msg or "not found" in error_msg:
            print(f"Download error: This video does not exist or could not be found")
            return {'status': 'error', 'message': "This video does not exist or could not be found"}
        else:
            print(f"Download error: {e}")
            return {'status': 'error', 'message': str(e)}
    except Exception as e:
        print(f"Error downloading audio: {e}")
        return {'status': 'error', 'message': str(e)}
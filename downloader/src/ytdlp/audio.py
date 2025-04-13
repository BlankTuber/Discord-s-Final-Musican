import os
import yt_dlp
from ytdlp import utils

def download(url, download_path, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    try:
        with yt_dlp.YoutubeDL({'skip_download': True, 'quiet': True}) as ydl:
            info = ydl.extract_info(url, download=False)
            
            if not info:
                return None
            
            if not allow_live and info.get('duration') is None:
                print(f"Skipping: Content is a live stream (duration is None)")
                return None
                
            if max_duration_seconds and info.get('duration', 0) > max_duration_seconds:
                print(f"Skipping: Duration ({info.get('duration')}s) exceeds limit ({max_duration_seconds}s)")
                return None
                
            if max_size_mb and info.get('filesize_approx', 0) > max_size_mb * 1024 * 1024:
                print(f"Skipping: Estimated size ({info.get('filesize_approx') / (1024*1024):.1f}MB) exceeds limit ({max_size_mb}MB)")
                return None
            
            filename = utils.get_sanitized_filename(info)
            full_path = os.path.join(download_path, filename)
            
            if utils.file_exists(full_path):
                print(f"File already exists: {filename}")
                return {
                    'title': info.get('title', 'Unknown'),
                    'filename': full_path,
                    'duration': info.get('duration'),
                    'file_size': utils.get_file_size(full_path),
                    'platform': platform,
                    'skipped': True
                }
            
            ydl_opts = {
                'format': 'bestaudio/best',
                'postprocessors': [{
                    'key': 'FFmpegExtractAudio',
                    'preferredcodec': 'mp3',
                    'preferredquality': '192',
                }],
                'outtmpl': os.path.join(download_path, '%(id)s_%(title)s'),
                'progress_hooks': [utils.progress_hook],
                'ignoreerrors': False,
            }
            
            if max_duration_seconds or max_size_mb:
                ydl_opts['match_filter'] = lambda info: utils.match_filter_func(
                    info, max_duration_seconds, max_size_mb
                )
            
            info = yt_dlp.YoutubeDL(ydl_opts).extract_info(url, download=True)
            
            if not info:
                return None
            
            downloaded_file = os.path.join(download_path, f"{info['id']}_{info['title']}.mp3")
            
            utils.rename_file(downloaded_file, full_path)
            
            return {
                'title': info.get('title', 'Unknown'),
                'filename': full_path,
                'duration': info.get('duration'),
                'file_size': utils.get_file_size(full_path),
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
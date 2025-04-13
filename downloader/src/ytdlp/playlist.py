import os
import yt_dlp
from ytdlp import utils

def download(url, download_path, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    
    try:
        with yt_dlp.YoutubeDL({'skip_download': True, 'quiet': True, 'noplaylist': False}) as ydl:
            info = ydl.extract_info(url, download=False)
            
            if not info or not info.get('entries'):
                print("No items found in playlist or not a playlist URL")
                return None
            
            playlist_title = info.get('title', 'Unknown Playlist')
            playlist_id = info.get('id', '')
            playlist_dir = f"{playlist_id}_{playlist_title}"
            playlist_path = os.path.join(download_path, playlist_dir)
            utils.ensure_directory(playlist_path)
            
            results = []
            for i, entry in enumerate(info.get('entries', [])):
                if not entry:
                    continue
                    
                if max_items and i >= max_items:
                    break
                    
                if not allow_live and entry.get('duration') is None:
                    print(f"Skipping playlist item: Live stream detected")
                    continue
                    
                if max_duration_seconds and entry.get('duration', 0) > max_duration_seconds:
                    print(f"Skipping playlist item: Duration too long")
                    continue
                    
                if max_size_mb and entry.get('filesize_approx', 0) > max_size_mb * 1024 * 1024:
                    print(f"Skipping playlist item: File too large")
                    continue
                
                filename = utils.get_sanitized_filename(entry)
                track_number = f"{i+1:02d}"
                final_filename = f"{track_number}_{filename}"
                full_path = os.path.join(playlist_path, final_filename)
                
                if utils.file_exists(full_path):
                    print(f"File already exists: {final_filename}")
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'filename': full_path,
                        'duration': entry.get('duration'),
                        'file_size': utils.get_file_size(full_path),
                        'platform': platform,
                        'skipped': True
                    })
                    continue
                
                ydl_opts = {
                    'format': 'bestaudio/best',
                    'postprocessors': [{
                        'key': 'FFmpegExtractAudio',
                        'preferredcodec': 'mp3',
                        'preferredquality': '192',
                    }],
                    'outtmpl': os.path.join(playlist_path, f"{track_number}_%(id)s_%(title)s"),
                    'progress_hooks': [utils.progress_hook],
                    'ignoreerrors': False,
                }
                
                try:
                    item_info = yt_dlp.YoutubeDL(ydl_opts).extract_info(entry['webpage_url'], download=True)
                    
                    if not item_info:
                        continue
                        
                    downloaded_file = os.path.join(playlist_path, f"{track_number}_{item_info['id']}_{item_info['title']}.mp3")
                    
                    utils.rename_file(downloaded_file, full_path)
                    
                    results.append({
                        'title': item_info.get('title', 'Unknown'),
                        'filename': full_path,
                        'duration': item_info.get('duration'),
                        'file_size': utils.get_file_size(full_path),
                        'platform': platform
                    })
                except Exception as e:
                    print(f"Error downloading playlist item: {e}")
            
            return {
                'playlist_title': playlist_title,
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
import os
import time
import yt_dlp
from ytdlp import utils

def download(url, download_path, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    platform_prefix = utils.get_platform_prefix(platform)
    
    try:
        # Use socket_timeout and extract_flat for faster extraction and to avoid freezing
        with yt_dlp.YoutubeDL({
            'skip_download': True, 
            'quiet': True, 
            'noplaylist': False,
            'extract_flat': True,
            'socket_timeout': 30,
            'ignoreerrors': True
        }) as ydl:
            info = ydl.extract_info(url, download=False)
            
            if not info or not info.get('entries'):
                print("No items found in playlist or not a playlist URL")
                raise yt_dlp.utils.DownloadError("No items found in playlist or not a playlist URL")
            
            playlist_title = info.get('title', 'Unknown Playlist')
            playlist_id = info.get('id', '')
            
            results = []
            entries = list(info.get('entries', []))
            
            # Filter out entries that are known to be unavailable
            filtered_entries = []
            for entry in entries:
                if entry and entry.get('id') is not None:
                    filtered_entries.append(entry)
                else:
                    print(f"Skipping unavailable video in playlist")
            
            entries = filtered_entries
            
            if max_items:
                entries = entries[:max_items]
            
            if not entries:
                print("No available videos found in playlist")
                return {
                    'playlist_title': playlist_title,
                    'playlist_url': url,
                    'count': 0,
                    'items': []
                }
            
            for i, entry in enumerate(entries):
                if not entry or not entry.get('id'):
                    print(f"Skipping unavailable playlist item")
                    continue
                
                video_id = entry.get('id')
                
                # Use the same naming scheme as single audio downloads
                filename = f"{platform_prefix}_{video_id}.mp3"
                full_path = os.path.abspath(os.path.join(download_path, filename))
                
                if os.path.isfile(full_path):
                    print(f"File already exists: {filename}")
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'filename': full_path,
                        'duration': entry.get('duration'),
                        'file_size': os.path.getsize(full_path),
                        'platform': platform,
                        'skipped': True
                    })
                    continue
                
                # Get full info for the specific video with timeout
                try:
                    with yt_dlp.YoutubeDL({
                        'skip_download': True, 
                        'quiet': True,
                        'socket_timeout': 15
                    }) as info_ydl:
                        video_info = info_ydl.extract_info(
                            f"https://www.youtube.com/watch?v={video_id}", 
                            download=False
                        )
                        
                        if not video_info:
                            print(f"Skipping unavailable playlist item: {entry.get('title', 'Unknown')}")
                            results.append({
                                'title': entry.get('title', 'Unknown'),
                                'filename': None,
                                'duration': None,
                                'file_size': None,
                                'platform': platform,
                                'skipped': True,
                                'error': "Video is unavailable"
                            })
                            continue
                        
                        if not allow_live and video_info.get('duration') is None:
                            print(f"Skipping playlist item: Live stream detected")
                            results.append({
                                'title': video_info.get('title', 'Unknown'),
                                'filename': None,
                                'duration': None,
                                'file_size': None,
                                'platform': platform,
                                'skipped': True,
                                'error': "Live stream detected"
                            })
                            continue
                            
                        if max_duration_seconds and video_info.get('duration', 0) > max_duration_seconds:
                            print(f"Skipping playlist item: Duration too long")
                            results.append({
                                'title': video_info.get('title', 'Unknown'),
                                'filename': None,
                                'duration': video_info.get('duration'),
                                'file_size': None,
                                'platform': platform,
                                'skipped': True,
                                'error': "Duration exceeds limit"
                            })
                            continue
                            
                        if max_size_mb and video_info.get('filesize_approx', 0) > max_size_mb * 1024 * 1024:
                            print(f"Skipping playlist item: File too large")
                            results.append({
                                'title': video_info.get('title', 'Unknown'),
                                'filename': None,
                                'duration': video_info.get('duration'),
                                'file_size': None,
                                'platform': platform,
                                'skipped': True,
                                'error': "File size exceeds limit"
                            })
                            continue
                            
                except Exception as e:
                    print(f"Error checking playlist item {video_id}: {e}")
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'filename': None,
                        'duration': None,
                        'file_size': None,
                        'platform': platform,
                        'skipped': True,
                        'error': f"Error checking video: {str(e)}"
                    })
                    continue
                
                # Actual download with timeout
                ydl_opts = {
                    'format': 'bestaudio/best',
                    'postprocessors': [{
                        'key': 'FFmpegExtractAudio',
                        'preferredcodec': 'mp3',
                        'preferredquality': '192',
                    }],
                    # Use the same output template as single audio downloads
                    'outtmpl': os.path.join(download_path, f"{platform_prefix}_%(id)s.%(ext)s"),
                    'progress_hooks': [utils.progress_hook],
                    'socket_timeout': 30,
                    'ignoreerrors': True,  # Don't stop on download errors
                    'retries': 3,          # Retry a few times
                    'fragment_retries': 3, # Retry fragment downloads
                    'extractor_retries': 3 # Retry extractor fetch
                }
                
                try:
                    with yt_dlp.YoutubeDL(ydl_opts) as item_ydl:
                        print(f"Downloading playlist item {i+1}/{len(entries)}: {entry.get('title', 'Unknown')}")
                        item_info = item_ydl.extract_info(f"https://www.youtube.com/watch?v={video_id}", download=True)
                        
                        if not item_info:
                            print(f"Failed to download playlist item: {entry.get('title', 'Unknown')}")
                            results.append({
                                'title': entry.get('title', 'Unknown'),
                                'filename': None,
                                'duration': None,
                                'file_size': None,
                                'platform': platform,
                                'skipped': True,
                                'error': "Download failed"
                            })
                            continue
                        
                        filename = f"{platform_prefix}_{item_info['id']}.mp3"
                        full_path = os.path.join(download_path, filename)
                        
                        file_exists = os.path.exists(full_path)
                        file_size = os.path.getsize(full_path) if file_exists else None
                        
                        results.append({
                            'title': item_info.get('title', 'Unknown'),
                            'filename': full_path,
                            'duration': item_info.get('duration'),
                            'file_size': file_size,
                            'platform': platform,
                            'skipped': False
                        })
                except yt_dlp.utils.DownloadError as e:
                    error_msg = str(e).lower()
                    error_message = "Error downloading playlist item"
                    
                    if "private" in error_msg:
                        error_message = "This video is private"
                    elif any(term in error_msg for term in ["premium", "paywall", "subscribe", "login", "member", "paid"]):
                        error_message = "This content requires a premium account or login"
                    elif any(term in error_msg for term in ["removed", "deleted", "taken down"]):
                        error_message = "This video has been removed or deleted"
                    elif "unavailable" in error_msg:
                        error_message = "This video is unavailable"
                    elif "copyright" in error_msg:
                        error_message = "This video is blocked due to copyright issues"
                    elif "age" in error_msg and ("restrict" in error_msg or "verify" in error_msg):
                        error_message = "This video is age-restricted"
                    elif ("geo" in error_msg and "block" in error_msg) or "country" in error_msg:
                        error_message = "This video is not available in your country"
                    elif "not exist" in error_msg or "no longer" in error_msg or "not found" in error_msg:
                        error_message = "This video does not exist or could not be found"
                    
                    print(f"Error downloading playlist item: {error_message}")
                    
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'filename': None,
                        'duration': None,
                        'file_size': None,
                        'platform': platform,
                        'skipped': True,
                        'error': error_message
                    })
                except Exception as e:
                    print(f"Error downloading playlist item: {e}")
                    
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'filename': None,
                        'duration': None,
                        'file_size': None,
                        'platform': platform,
                        'skipped': True,
                        'error': str(e)
                    })
            
            return {
                'playlist_title': playlist_title,
                'playlist_url': url,
                'count': len(results),
                'items': results,
                'successful_downloads': sum(1 for item in results if not item.get('skipped', True))
            }
    except yt_dlp.utils.DownloadError as e:
        error_msg = str(e).lower()
        
        if "private" in error_msg:
            print(f"Download error: This playlist is private")
            raise yt_dlp.utils.DownloadError("This playlist is private")
        elif any(term in error_msg for term in ["premium", "paywall", "subscribe", "login", "member", "paid"]):
            print(f"Download error: This playlist requires a premium account or login")
            raise yt_dlp.utils.DownloadError("This playlist requires a premium account or login")
        elif "unavailable" in error_msg:
            print(f"Download error: This playlist is unavailable")
            raise yt_dlp.utils.DownloadError("This playlist is unavailable")
        elif "not exist" in error_msg or "no longer" in error_msg or "not found" in error_msg:
            print(f"Download error: This playlist does not exist or could not be found")
            raise yt_dlp.utils.DownloadError("This playlist does not exist or could not be found")
        else:
            print(f"Download error: {e}")
            raise
    except Exception as e:
        print(f"Error downloading playlist: {e}")
        raise
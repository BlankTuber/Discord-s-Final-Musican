import os
import time
import yt_dlp
from ytdlp import utils

def download(url, download_path, db, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    platform_prefix = utils.get_platform_prefix(platform)
    
    try:
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
            
            playlist_dir = f"{platform_prefix}_{playlist_id}"
            playlist_path = os.path.join(download_path, playlist_dir)
            os.makedirs(playlist_path, exist_ok=True)
            
            results = []
            entries = list(info.get('entries', []))
            
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
            
            # Check song count to warn if we're approaching limit, but don't delete anything
            # Only janitor should delete files and DB entries
            song_count = db.get_song_count()
            if song_count + len(entries) > 500:
                print(f"Warning: Adding {len(entries)} songs would exceed the limit of 500 (current count: {song_count}).")
                print("The janitor will clean up old songs on its next run.")
            
            # Create playlist record in database ONLY if we successfully download at least one song
            db_playlist_id = None
            successful_downloads = 0
            
            for i, entry in enumerate(entries):
                if not entry or not entry.get('id'):
                    print(f"Skipping unavailable playlist item")
                    continue
                
                track_number = f"{i+1:02d}"
                video_id = entry.get('id')
                
                video_url = f"https://www.youtube.com/watch?v={video_id}"
                
                # Check if song exists in database and file exists
                song = db.get_song_by_url(video_url)
                if song and os.path.exists(song['file_path']):
                    print(f"Song already exists: {song['title']}")
                    
                    # Create playlist in database if not created yet
                    if db_playlist_id is None:
                        db_playlist = db.get_playlist_by_url(url)
                        if db_playlist:
                            db_playlist_id = db_playlist['id']
                        else:
                            db_playlist_id = db.add_playlist(
                                title=playlist_title,
                                url=url,
                                platform=platform
                            )
                    
                    # Add song to playlist if not already there
                    position_result = db.query(
                        "SELECT position FROM playlist_songs WHERE playlist_id = ? AND song_id = ?",
                        (db_playlist_id, song['id'])
                    )
                    
                    if not position_result:
                        db.add_song_to_playlist(db_playlist_id, song['id'], i)
                        print(f"Added existing song to playlist: {song['title']}")
                    
                    successful_downloads += 1
                    results.append({
                        'id': song['id'],
                        'title': song['title'],
                        'filename': song['file_path'],
                        'duration': song['duration'],
                        'file_size': song['file_size'],
                        'platform': song['platform'],
                        'skipped': True
                    })
                    continue
                
                filename = f"{track_number}_{platform_prefix}_{video_id}.mp3"
                full_path = os.path.abspath(os.path.join(playlist_path, filename))
                
                if os.path.isfile(full_path):
                    # File exists but not in database, will add it after checking info
                    file_exists = True
                    file_size = os.path.getsize(full_path)
                else:
                    file_exists = False
                    file_size = None
                
                try:
                    with yt_dlp.YoutubeDL({
                        'skip_download': True, 
                        'quiet': True,
                        'socket_timeout': 15
                    }) as info_ydl:
                        video_info = info_ydl.extract_info(video_url, download=False)
                        
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
                
                # If file exists, we can skip download
                if file_exists:
                    item_info = video_info
                else:
                    # Actually download the file
                    ydl_opts = {
                        'format': 'bestaudio/best',
                        'postprocessors': [{
                            'key': 'FFmpegExtractAudio',
                            'preferredcodec': 'mp3',
                            'preferredquality': '192',
                        }],
                        'outtmpl': os.path.join(playlist_path, f"{track_number}_{platform_prefix}_%(id)s.%(ext)s"),
                        'progress_hooks': [utils.progress_hook],
                        'socket_timeout': 30,
                        'ignoreerrors': True,
                        'retries': 3,
                        'fragment_retries': 3,
                        'extractor_retries': 3
                    }
                    
                    try:
                        with yt_dlp.YoutubeDL(ydl_opts) as item_ydl:
                            print(f"Downloading playlist item {i+1}/{len(entries)}: {entry.get('title', 'Unknown')}")
                            item_info = item_ydl.extract_info(video_url, download=True)
                            
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
                            
                            # Verify file exists after download
                            file_exists = os.path.exists(full_path)
                            if not file_exists:
                                print(f"Download completed but file not found: {full_path}")
                                results.append({
                                    'title': item_info.get('title', 'Unknown'),
                                    'filename': None,
                                    'duration': None,
                                    'file_size': None,
                                    'platform': platform,
                                    'skipped': True,
                                    'error': "File not found after download"
                                })
                                continue
                            
                            file_size = os.path.getsize(full_path)
                        
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
                        continue
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
                        continue
                
                # Create playlist in database if this is the first successful download
                if db_playlist_id is None:
                    db_playlist = db.get_playlist_by_url(url)
                    if db_playlist:
                        db_playlist_id = db_playlist['id']
                    else:
                        db_playlist_id = db.add_playlist(
                            title=playlist_title,
                            url=url,
                            platform=platform
                        )
                
                # At this point, the file is either already there or we've downloaded it successfully
                thumbnail = item_info.get('thumbnail', '')
                if isinstance(thumbnail, dict) and 'url' in thumbnail:
                    thumbnail = thumbnail['url']
                
                artist = item_info.get('artist', item_info.get('uploader', item_info.get('channel', 'Unknown')))
                
                # Now add to database
                song_id = db.add_song(
                    title=item_info.get('title', 'Unknown'),
                    url=video_url,
                    platform=platform,
                    file_path=full_path,
                    duration=item_info.get('duration'),
                    file_size=file_size,
                    thumbnail_url=thumbnail,
                    artist=artist,
                    is_stream=item_info.get('is_live', False)
                )
                
                db.add_song_to_playlist(db_playlist_id, song_id, i)
                successful_downloads += 1
                
                results.append({
                    'id': song_id,
                    'title': item_info.get('title', 'Unknown'),
                    'filename': full_path,
                    'duration': item_info.get('duration'),
                    'file_size': file_size,
                    'platform': platform,
                    'skipped': False
                })
            
            return {
                'playlist_title': playlist_title,
                'playlist_url': url,
                'count': len(results),
                'items': results,
                'successful_downloads': successful_downloads
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
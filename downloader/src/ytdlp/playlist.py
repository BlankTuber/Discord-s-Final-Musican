import os
import time
import yt_dlp
from ytdlp import utils, audio

def download(url, download_path, db, max_items=None, max_duration_seconds=None, max_size_mb=None, allow_live=False):
    platform = utils.get_platform(url)
    platform_prefix = utils.get_platform_prefix(platform)
    
    try:
        # First, check if the playlist exists in the database
        db_playlist = None
        try:
            db_playlist = db.get_playlist_by_url(url)
        except Exception as e:
            print(f"Error checking for existing playlist: {e}")
        
        # Get playlist info
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
            if db_playlist:
                db_playlist_id = db_playlist['id']
                print(f"Using existing playlist with ID: {db_playlist_id}")
            else:
                try:
                    db_playlist_id = db.add_playlist(
                        title=playlist_title,
                        url=url,
                        platform=platform
                    )
                    print(f"Created new playlist with ID: {db_playlist_id}")
                except Exception as e:
                    print(f"Error creating playlist in database: {e}")
            
            successful_downloads = 0
            
            # Process each entry in the playlist
            for i, entry in enumerate(entries):
                if not entry or not entry.get('id'):
                    print(f"Skipping unavailable playlist item")
                    continue
                
                video_id = entry.get('id')
                video_url = f"https://www.youtube.com/watch?v={video_id}"
                
                print(f"Processing item {i+1}/{len(entries)}: {entry.get('title', 'Unknown')}")
                
                # Use the audio download function with return_existing=True to avoid duplicating code
                try:
                    result = audio.download(
                        video_url, 
                        download_path, 
                        db,
                        max_duration_seconds=max_duration_seconds,
                        max_size_mb=max_size_mb,
                        allow_live=allow_live
                    )
                    
                    if not result:
                        print(f"Failed to download or process playlist item: {video_url}")
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
                    
                    # Add song to playlist if we have playlist ID
                    if db_playlist_id and 'id' in result:
                        song_id = result['id']
                        try:
                            # Check if song is already in playlist
                            position_result = db.query(
                                "SELECT position FROM playlist_songs WHERE playlist_id = ? AND song_id = ?",
                                (db_playlist_id, song_id)
                            )
                            
                            if not position_result:
                                db.add_song_to_playlist(db_playlist_id, song_id, i)
                                print(f"Added song ID {song_id} to playlist ID {db_playlist_id}")
                            else:
                                print(f"Song ID {song_id} already in playlist ID {db_playlist_id}")
                        except Exception as e:
                            print(f"Error adding song to playlist: {e}")
                    
                    successful_downloads += 1
                    results.append(result)
                
                except Exception as e:
                    print(f"Error processing playlist item {video_url}: {e}")
                    results.append({
                        'title': entry.get('title', 'Unknown'),
                        'filename': None,
                        'duration': None,
                        'file_size': None,
                        'platform': platform,
                        'skipped': True,
                        'error': str(e)
                    })
            
            # Wait a moment to ensure all database operations are complete
            time.sleep(0.5)
            
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
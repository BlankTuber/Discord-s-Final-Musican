import sqlite3
import os
import time
import threading

class Database:
    def __init__(self, db_path):
        self.db_path = db_path
        self.local = threading.local()
        
        # Check if database exists
        if not os.path.exists(db_path):
            print(f"Warning: Database file does not exist at: {db_path}")
            print("Will continue, but some functionality will be limited")
        
    def get_connection(self):
        if not hasattr(self.local, 'conn') or self.local.conn is None:
            try:
                self.local.conn = sqlite3.connect(self.db_path, timeout=10.0)
                self.local.conn.row_factory = sqlite3.Row
            except sqlite3.Error as e:
                print(f"Database connection error: {e}")
                raise
        return self.local.conn
        
    def close(self):
        if hasattr(self.local, 'conn') and self.local.conn:
            self.local.conn.close()
            self.local.conn = None
            
    def __enter__(self):
        return self
        
    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()
    
    def execute(self, query, params=None):
        attempts = 0
        max_attempts = 3
        last_error = None
        
        while attempts < max_attempts:
            try:
                conn = self.get_connection()
                cursor = conn.cursor()
                if params:
                    cursor.execute(query, params)
                else:
                    cursor.execute(query)
                conn.commit()
                return cursor
            except sqlite3.Error as e:
                last_error = e
                attempts += 1
                print(f"Database execute error (attempt {attempts}/{max_attempts}): {e}")
                
                # Close and reopen connection
                self.close()
                
                # Wait before retrying
                time.sleep(0.5)
        
        print(f"Failed all {max_attempts} database execute attempts")
        raise last_error
    
    def query(self, query, params=None):
        attempts = 0
        max_attempts = 3
        last_error = None
        
        while attempts < max_attempts:
            try:
                conn = self.get_connection()
                cursor = conn.cursor()
                if params:
                    cursor.execute(query, params)
                else:
                    cursor.execute(query)
                return cursor.fetchall()
            except sqlite3.Error as e:
                last_error = e
                attempts += 1
                print(f"Database query error (attempt {attempts}/{max_attempts}): {e}")
                
                # Close and reopen connection
                self.close()
                
                # Wait before retrying
                time.sleep(0.5)
        
        print(f"Failed all {max_attempts} database query attempts")
        raise last_error
    
    def get_song_by_url(self, url):
        try:
            result = self.query("SELECT * FROM songs WHERE url = ?", (url,))
            return result[0] if result else None
        except Exception as e:
            print(f"Error in get_song_by_url: {e}")
            return None
    
    def get_song_by_path(self, file_path):
        try:
            result = self.query("SELECT * FROM songs WHERE file_path = ?", (file_path,))
            return result[0] if result else None
        except Exception as e:
            print(f"Error in get_song_by_path: {e}")
            return None
    
    def get_playlist_by_url(self, url):
        try:
            result = self.query("SELECT * FROM playlists WHERE url = ?", (url,))
            return result[0] if result else None
        except Exception as e:
            print(f"Error in get_playlist_by_url: {e}")
            return None
    
    def add_song(self, title, url, platform, file_path, duration=None, file_size=None, 
                thumbnail_url=None, artist=None, is_stream=False):
        try:
            # First check if the song already exists
            existing = self.get_song_by_url(url)
            if existing:
                return existing['id']
            
            current_time = int(time.time())
            self.execute(
                """
                INSERT INTO songs (
                    title, url, platform, file_path, duration, file_size, 
                    thumbnail_url, artist, download_date, is_stream
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (title, url, platform, file_path, duration, file_size, 
                 thumbnail_url, artist, current_time, is_stream)
            )
            
            # Get the ID of the inserted row
            result = self.query("SELECT last_insert_rowid()")
            return result[0][0] if result else None
            
        except Exception as e:
            print(f"Error in add_song: {e}")
            raise
    
    def add_playlist(self, title, url, platform):
        try:
            # First check if the playlist already exists
            existing = self.get_playlist_by_url(url)
            if existing:
                return existing['id']
            
            current_time = int(time.time())
            self.execute(
                "INSERT INTO playlists (title, url, platform, download_date) VALUES (?, ?, ?, ?)",
                (title, url, platform, current_time)
            )
            
            # Get the ID of the inserted row
            result = self.query("SELECT last_insert_rowid()")
            return result[0][0] if result else None
            
        except Exception as e:
            print(f"Error in add_playlist: {e}")
            raise
    
    def add_song_to_playlist(self, playlist_id, song_id, position):
        try:
            # Check if the song is already in the playlist
            result = self.query(
                "SELECT position FROM playlist_songs WHERE playlist_id = ? AND song_id = ?",
                (playlist_id, song_id)
            )
            
            if result:
                # Song already in playlist, update position if needed
                if result[0]['position'] != position:
                    self.execute(
                        "UPDATE playlist_songs SET position = ? WHERE playlist_id = ? AND song_id = ?",
                        (position, playlist_id, song_id)
                    )
                return
            
            # Song not in playlist, add it
            self.execute(
                "INSERT INTO playlist_songs (playlist_id, song_id, position) VALUES (?, ?, ?)",
                (playlist_id, song_id, position)
            )
            
        except Exception as e:
            print(f"Error in add_song_to_playlist: {e}")
            raise
    
    def increment_play_count(self, song_id):
        try:
            current_time = int(time.time())
            self.execute(
                "UPDATE songs SET play_count = play_count + 1, last_played = ? WHERE id = ?",
                (current_time, song_id)
            )
        except Exception as e:
            print(f"Error in increment_play_count: {e}")
    
    def get_song_count(self):
        try:
            result = self.query("SELECT COUNT(*) FROM songs")
            return result[0][0] if result else 0
        except Exception as e:
            print(f"Error in get_song_count: {e}")
            return 0
    
    def get_least_popular_songs(self, limit):
        try:
            return self.query(
                """
                SELECT * FROM songs
                ORDER BY play_count ASC, last_played ASC
                LIMIT ?
                """,
                (limit,)
            )
        except Exception as e:
            print(f"Error in get_least_popular_songs: {e}")
            return []
    
    def delete_song(self, song_id):
        try:
            self.execute("DELETE FROM playlist_songs WHERE song_id = ?", (song_id,))
            self.execute("DELETE FROM songs WHERE id = ?", (song_id,))
        except Exception as e:
            print(f"Error in delete_song: {e}")
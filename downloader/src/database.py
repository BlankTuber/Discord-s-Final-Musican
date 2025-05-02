import sqlite3
import os
import time
import threading

class Database:
    def __init__(self, db_path):
        self.db_path = db_path
        self.local = threading.local()
        
    def get_connection(self):
        if not hasattr(self.local, 'conn') or self.local.conn is None:
            try:
                self.local.conn = sqlite3.connect(self.db_path)
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
            print(f"Database execute error: {e}")
            raise
    
    def query(self, query, params=None):
        try:
            conn = self.get_connection()
            cursor = conn.cursor()
            if params:
                cursor.execute(query, params)
            else:
                cursor.execute(query)
            return cursor.fetchall()
        except sqlite3.Error as e:
            print(f"Database query error: {e}")
            raise
    
    def get_song_by_url(self, url):
        result = self.query("SELECT * FROM songs WHERE url = ?", (url,))
        return result[0] if result else None
    
    def get_song_by_path(self, file_path):
        result = self.query("SELECT * FROM songs WHERE file_path = ?", (file_path,))
        return result[0] if result else None
    
    def get_playlist_by_url(self, url):
        result = self.query("SELECT * FROM playlists WHERE url = ?", (url,))
        return result[0] if result else None
    
    def add_song(self, title, url, platform, file_path, duration=None, file_size=None, 
                thumbnail_url=None, artist=None, is_stream=False):
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
        return self.query("SELECT last_insert_rowid()")[0][0]
    
    def add_playlist(self, title, url, platform):
        current_time = int(time.time())
        self.execute(
            "INSERT INTO playlists (title, url, platform, download_date) VALUES (?, ?, ?, ?)",
            (title, url, platform, current_time)
        )
        return self.query("SELECT last_insert_rowid()")[0][0]
    
    def add_song_to_playlist(self, playlist_id, song_id, position):
        self.execute(
            "INSERT INTO playlist_songs (playlist_id, song_id, position) VALUES (?, ?, ?)",
            (playlist_id, song_id, position)
        )
    
    def increment_play_count(self, song_id):
        current_time = int(time.time())
        self.execute(
            "UPDATE songs SET play_count = play_count + 1, last_played = ? WHERE id = ?",
            (current_time, song_id)
        )
    
    def get_song_count(self):
        result = self.query("SELECT COUNT(*) FROM songs")
        return result[0][0] if result else 0
    
    def get_least_popular_songs(self, limit):
        return self.query(
            """
            SELECT * FROM songs
            ORDER BY play_count ASC, last_played ASC
            LIMIT ?
            """,
            (limit,)
        )
    
    def delete_song(self, song_id):
        self.execute("DELETE FROM playlist_songs WHERE song_id = ?", (song_id,))
        self.execute("DELETE FROM songs WHERE id = ?", (song_id,))
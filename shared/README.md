# Shared folder

-   Contains downloaded music files
-   Houses the SQLite database file
-   Includes database initialization tool

## Database Setup

Before running any component, you need to initialize the database:

```bash
# Navigate to the shared folder
cd shared

# Build the initializer
go build -o db_init db_initializer.go

# Run the initializer with the correct path
./db_init -path .

# This ensures the database is created in the shared folder
```

This will create the `musicbot.db` file with all required tables in the shared folder.

## Database Schema

The database contains the following tables:

### Songs Table

-   `id`: Primary key
-   `title`: Song title
-   `url`: Original URL (unique)
-   `platform`: Source platform
-   `file_path`: Path to the MP3 file
-   `duration`: Song duration in seconds
-   `file_size`: File size in bytes
-   `thumbnail_url`: URL to thumbnail
-   `artist`: Artist name
-   `download_date`: Unix timestamp of download
-   `play_count`: Number of times played
-   `last_played`: Unix timestamp of last play
-   `is_stream`: Boolean flag for streams

### Playlists Table

-   `id`: Primary key
-   `title`: Playlist title
-   `url`: Original URL (unique)
-   `platform`: Source platform
-   `download_date`: Unix timestamp of download

### Playlist_Songs Table

-   `playlist_id`: Foreign key to playlists.id
-   `song_id`: Foreign key to songs.id
-   `position`: Song position in playlist

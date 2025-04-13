# UDS JSON Protocol Examples

## Ping Example

### Request

```json
{
    "command": "ping",
    "id": "ping-123",
    "params": {
        "timestamp": "2023-04-13T14:32:10.123Z"
    }
}
```

### Response

```json
{
    "status": "success",
    "id": "ping-123",
    "data": {
        "message": "pong",
        "timestamp": "2023-04-13T14:32:10.123Z"
    },
    "timestamp": "2023-04-13T14:32:10.456Z"
}
```

## Download Audio Example

### Request

```json
{
    "command": "download_audio",
    "id": "download-456",
    "params": {
        "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
        "max_duration_seconds": 600,
        "max_size_mb": 50,
        "allow_live": false
    }
}
```

### Success Response

```json
{
    "status": "success",
    "id": "download-456",
    "data": {
        "title": "Rick Astley - Never Gonna Give You Up (Official Music Video)",
        "filename": "/path/to/download/dQw4w9WgXcQ.mp3",
        "duration": 213,
        "file_size": 3407872,
        "platform": "https://youtube.com"
    },
    "timestamp": "2023-04-13T14:33:25.789Z"
}
```

### Error Response

```json
{
    "status": "error",
    "id": "download-456",
    "error": "Download failed: Video unavailable",
    "timestamp": "2023-04-13T14:33:15.123Z"
}
```

## Download Playlist Example

### Request

```json
{
    "command": "download_playlist",
    "id": "playlist-789",
    "params": {
        "url": "https://www.youtube.com/playlist?list=PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI",
        "max_items": 5,
        "max_duration_seconds": 600,
        "max_size_mb": 50,
        "allow_live": false
    }
}
```

### Success Response

```json
{
    "status": "success",
    "id": "playlist-789",
    "data": {
        "playlist_title": "Lo-Fi Beats for Studying",
        "playlist_url": "https://www.youtube.com/playlist?list=PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI",
        "count": 3,
        "items": [
            {
                "title": "Lo-Fi Beat 1",
                "filename": "/path/to/download/PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI/01_abcd1234.mp3",
                "duration": 183,
                "file_size": 2945024,
                "platform": "https://youtube.com"
            },
            {
                "title": "Lo-Fi Beat 2",
                "filename": "/path/to/download/PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI/02_efgh5678.mp3",
                "duration": 210,
                "file_size": 3276800,
                "platform": "https://youtube.com"
            },
            {
                "title": "Lo-Fi Beat 3",
                "filename": "/path/to/download/PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI/03_ijkl9012.mp3",
                "duration": 195,
                "file_size": 3072000,
                "platform": "https://youtube.com",
                "skipped": true
            }
        ]
    },
    "timestamp": "2023-04-13T14:40:12.345Z"
}
```

## Search Example

### Request

```json
{
    "command": "search",
    "id": "search-012",
    "params": {
        "query": "lofi beats",
        "platform": "youtube",
        "limit": 3,
        "include_live": false
    }
}
```

### Success Response

```json
{
    "status": "success",
    "id": "search-012",
    "data": {
        "results": [
            {
                "title": "lofi hip hop radio - beats to relax/study to",
                "url": "https://www.youtube.com/watch?v=jfKfPfyJRdk",
                "duration": 7245,
                "uploader": "Lofi Girl",
                "thumbnail": "https://i.ytimg.com/vi/jfKfPfyJRdk/hqdefault.jpg",
                "platform": "https://youtube.com"
            },
            {
                "title": "Chill Lo-Fi Hip-Hop Beats Free To Use",
                "url": "https://www.youtube.com/watch?v=q5xIoeG4uVI",
                "duration": 1234,
                "uploader": "Music Producer Channel",
                "thumbnail": "https://i.ytimg.com/vi/q5xIoeG4uVI/hqdefault.jpg",
                "platform": "https://youtube.com"
            },
            {
                "title": "1 Hour of Relaxing Lo-Fi Music",
                "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
                "duration": 3600,
                "uploader": "Chill Music Channel",
                "thumbnail": "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg",
                "platform": "https://youtube.com"
            }
        ]
    },
    "timestamp": "2023-04-13T14:45:05.678Z"
}
```

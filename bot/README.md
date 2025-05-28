# Discord Music Bot

A modular Discord bot for streaming radio and playing music in voice channels with automatic idle mode.

## Features

- **Idle Mode**: Automatically joins a designated idle voice channel and plays radio
- **Smart Voice Management**: Automatically follows users and returns to idle when alone
- **Multiple Radio Streams**: Support for multiple predefined radio streams
- **Music Playback**: Download and play songs from URLs with queue management
- **Playlist Support**: Download entire playlists and add them to the queue
- **Music Search**: Search for songs and select from results
- **State Management**: Thread-safe state tracking and operation prevention
- **Graceful Shutdown**: Coordinated shutdown sequence for all components
- **Connection Stability**: Robust error handling and automatic reconnection
- **Stream Resilience**: Handles timeouts, rate limits, and connection issues
- **Smart Command Updates**: Only updates Discord commands when they actually change
- **Modular Architecture**: Clean, reusable components for easy extension

## Commands

### Voice Commands

- `/join` - Join your current voice channel
- `/leave` - Leave current channel (returns to idle)

### Radio Commands

- `/changestream <stream>` - Change the radio stream

### Music Commands

- `/play <url>` - Download and play a song from URL
- `/playlist <url>` - Download and queue an entire playlist
- `/search <query>` - Search for songs and select from results
- `/queue` - Show the current music queue
- `/nowplaying` - Show what's currently playing
- `/skip` - Skip the current song
- `/clear` - Clear the music queue

## Bot States

The bot operates in three main states:

1. **Idle State**: Playing radio in the designated idle channel
2. **Radio State**: Playing radio in a user channel  
3. **DJ State**: Playing music from the queue

## Setup

1. **Prerequisites**:
   - Go 1.24.1 or later
   - FFmpeg installed and in PATH
   - A Discord bot token
   - Opus libraries for audio encoding
   - Access to the `../janitor/janitor` executable
   - Access to the `../shared/` directory
   - Access to a downloader service (for music functionality)

2. **Configuration**:

   ```bash
   # Copy example config and edit with your settings
   cp config.json.example config.json
   # Edit config.json with your bot token, guild ID, and idle channel ID
   ```

3. **Build and Run**:

   ```bash
   make deps
   make build
   make run
   ```

## Quick Start

```bash
# Install dependencies
make deps

# Run in development mode with debug logging
make dev

# Or run normally
make run
```

## Configuration

### config.json

```json
{
  "token": "your_bot_token",
  "uds_path": "/tmp/downloader.sock",
  "guild_id": "your_guild_id",
  "idle_channel": "voice_channel_id",
  "db_path": "bot.db"
}
```

### Getting Required IDs

**Guild ID**:

1. Enable Developer Mode in Discord (User Settings → Advanced → Developer Mode)
2. Right-click your server → Copy Server ID

**Voice Channel ID**:

1. With Developer Mode enabled
2. Right-click the voice channel → Copy Channel ID

**Bot Token**:

1. Go to <https://discord.com/developers/applications>
2. Create/select your application → Bot → Copy Token

**Note**: The guild ID and idle channel ID must match (the channel must be in that guild).

### Available Streams

- `listen.moe` - Japanese music
- `listen.moe (kpop)` - Korean pop music  
- `lofi` - Lofi hip hop

## Architecture

```md
internal/
├── config/          # Configuration management and database
├── discord/         
│   ├── commands/    # Command routing and versioning
│   └── ...          # Discord client and events
├── voice/           # Voice connection management
├── radio/           # Radio streaming with resilience
├── music/           # Music playback and queue management
├── state/           # State management
├── socket/          # Socket communication with downloader
├── shutdown/        # Graceful shutdown coordination
└── logger/          # Logging utilities
```

## Development

```bash
# Run with debug logging to see command versioning
make dev

# Format code
make fmt

# Run tests
make test

# Clean build files
make clean

# Force command refresh (delete hash file)
rm command_hashes.json && make run
```

## Music Features

### Queue Management

- Songs are persistently stored in the database
- Queue position is maintained across bot restarts
- Automatic playback continues until queue is empty
- Smart state transitions between radio and music modes

### Download Integration

The bot communicates with a separate downloader service via Unix socket:

- **Download**: Single song download via `/play` command
- **Playlist**: Bulk playlist download via `/playlist` command  
- **Search**: Song search with interactive button selection

### State Transitions

1. **Radio → DJ**: When a song is added to the queue
2. **DJ → Radio**: When queue is empty or bot returns to idle
3. **Idle Management**: Bot returns to idle channel when left alone

## Bot Behavior

1. **On Startup**:
   - Runs janitor to clean up files
   - Connects to socket if available
   - Loads saved queue from database
   - Checks for command changes and updates only modified commands
   - Joins idle channel and starts radio

2. **Music Playback**:
   - Stops radio when music is requested
   - Automatically plays next song when current finishes
   - Returns to radio when queue is empty
   - Handles user channel changes intelligently

3. **User Interaction**:
   - Can use `/join` to move bot to their channel
   - Can add songs via `/play`, `/playlist`, or `/search`
   - Can manage queue with `/skip`, `/queue`, `/clear`

4. **Auto-Management**:
   - Returns to idle if channel becomes empty
   - Automatically reconnects to idle channel with retry logic
   - Prevents concurrent voice operations
   - Handles stream issues with automatic retry

5. **Graceful Shutdown**:
   - Stops music/radio playback first
   - Disconnects from voice channels
   - Closes Discord connection
   - Disconnects from socket
   - Saves queue state to database

## Requirements

- Discord bot with voice permissions
- Access to designated idle voice channel
- FFmpeg for audio processing
- Network access for radio streams
- Downloader service for music functionality (optional)

## Troubleshooting

**"Guild ID is required in config.json"**:

- Add your guild ID to the config.json file
- Make sure guild_id and idle_channel match (channel must be in that guild)

**"Janitor failed"**:

- Ensure `../janitor/janitor` executable exists
- Ensure `../shared/` directory exists  
- Check permissions on both paths

**"Failed to connect to Discord"**:

- Verify bot token in config.json
- Check bot permissions in Discord server

**"Failed to join idle channel"**:

- Verify both guild_id and idle_channel in config.json
- Ensure bot has voice permissions in that channel
- Make sure the channel belongs to the specified guild

**"Downloader not available"**:

- Check if downloader service is running
- Verify socket path in config.json
- Music commands will not work without downloader

**Music files not found**:

- Ensure `../shared/` directory exists and is writable
- Check that downloader saves files to correct location
- Verify file permissions

**Radio stream keeps cutting out**:

- Check network connectivity
- Bot automatically retries with smart error classification
- Monitor logs for specific error messages

**Bot disconnects randomly**:

- Enhanced stability with retry logic
- Check Discord API status
- Verify network stability

**Commands not updating properly**:

- Delete `command_hashes.json` to force full command refresh
- Check Discord API permissions for application commands
- Look for error messages in command update logs

**Queue not persisting**:

- Check database file permissions
- Verify database initialization in logs
- Database file is created automatically if missing

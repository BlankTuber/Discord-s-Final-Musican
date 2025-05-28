# Discord Music Bot

A modular Discord bot for streaming radio in voice channels with automatic idle mode.

## Features

- **Idle Mode**: Automatically joins a designated idle voice channel and plays radio
- **Smart Voice Management**: Automatically follows users and returns to idle when alone
- **Multiple Radio Streams**: Support for multiple predefined radio streams
- **State Management**: Thread-safe state tracking and operation prevention
- **Graceful Shutdown**: Coordinated shutdown sequence for all components
- **Connection Stability**: Robust error handling and automatic reconnection
- **Stream Resilience**: Handles timeouts, rate limits, and connection issues
- **Smart Command Updates**: Only updates Discord commands when they actually change
- **Modular Architecture**: Clean, reusable components for easy extension

## Commands

- `/join` - Join your current voice channel
- `/leave` - Leave current channel (returns to idle)
- `/changestream <stream>` - Change the radio stream

## Setup

1. **Prerequisites**:
   - Go 1.24.1 or later
   - FFmpeg installed and in PATH
   - A Discord bot token
   - Opus libraries for audio encoding
   - Access to the `../janitor/janitor` executable
   - Access to the `../shared/` directory

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
├── config/          # Configuration management
├── discord/         
│   ├── commands/    # Command routing and versioning
│   └── ...          # Discord client and events
├── voice/           # Voice connection management
├── radio/           # Radio streaming with resilience
├── state/           # State management
├── socket/          # Socket communication
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

## Command Versioning Example

**First run (no hash file exists):**

```md
INFO: Command hash file doesn't exist, will create: command_hashes.json
INFO: Checking for command changes...
INFO: Command changes detected:
INFO:   - Creating 3 new commands
INFO:     + join
INFO:     + leave  
INFO:     + changestream
INFO: Applying command changes...
INFO: Creating command: join
INFO: Creating command: leave
INFO: Creating command: changestream
INFO: Saved command registry (version 1)
```

**Subsequent runs (no changes):**

```md
INFO: Loaded command registry with 3 commands (version 1)
INFO: Checking for command changes...
INFO: No command changes detected
INFO: All commands are up to date
```

**When you modify a command:**

```md
INFO: Loaded command registry with 3 commands (version 1)  
INFO: Checking for command changes...
INFO: Command changes detected:
INFO:   - Updating 1 commands
INFO:     ~ changestream
INFO: Applying command changes...
INFO: Updating command: changestream
INFO: Saved command registry (version 2)
```

## Bot Behavior

1. **On Startup**:
   - Runs janitor to clean up files
   - Connects to socket if available
   - Checks for command changes and updates only modified commands
   - Joins idle channel and starts radio
2. **User Joins**: Can use `/join` to move bot to their channel
3. **User Leaves**: Bot returns to idle if channel becomes empty
4. **Disconnection**: Automatically reconnects to idle channel with retry logic
5. **State Conflicts**: Prevents concurrent voice operations
6. **Stream Issues**: Automatic retry with smart error classification
7. **Command Management**: Only updates Discord commands when definitions change
8. **Graceful Shutdown**:
   - Stops radio stream first
   - Disconnects from voice channels
   - Closes Discord connection
   - Disconnects from socket
   - Shuts down cleanly within 30 seconds

## Requirements

- Discord bot with voice permissions
- Access to designated idle voice channel
- FFmpeg for audio processing
- Network access for radio streams

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

**Shutdown takes too long**:

- Default timeout is 30 seconds
- Components shut down in priority order
- Check logs for which component is hanging

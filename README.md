# Discord's Final Musican 🎵🤖

A multi-component Discord music bot that automatically switches to radio mode when idle.

## Overview

This project consists of three main components:

-   🎮 **Discord Bot** - Handles commands and streams music in voice channels
-   📥 **Downloader** - Downloads music from YouTube and other platforms
-   🧹 **Janitor** - Cleans up old files to manage disk space

## Features

-   📻 Automatic radio mode when idle
-   🔊 Audio streaming from various sources
-   🔍 YouTube search functionality
-   📋 Playlist support

## Components Explained

### Discord Bot (Go)

Handles Discord integration, slash commands, and audio streaming. When left idle, it automatically joins a configured voice channel and streams music.

### Downloader (Python)

Powered by yt-dlp, this service downloads audio files from platforms like YouTube and SoundCloud. It communicates with the bot via a Unix Domain Socket.

### Janitor (C)

A C program that cleans up mp3 files so that the server does not get filled up

## Getting Started

1. Configure the bot settings in `bot/config.json`
2. Set up the downloader in `downloader/config/config.json`
3. Run all three components
4. Enjoy your music!

---

🎵 Enjoy your Discord music experience! 🎵

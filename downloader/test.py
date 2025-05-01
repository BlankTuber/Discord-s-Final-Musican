#!/usr/bin/env python3

import sys
sys.dont_write_bytecode = True  # Prevent __pycache__ creation

import os
import json
import argparse
import socket
import struct
import uuid
import time
import pprint
from datetime import datetime, timezone, UTC  # Import UTC for timezone

# Check for rich library, use fallback if not available
try:
    from rich.console import Console
    from rich.table import Table
    from rich.prompt import Prompt, Confirm
    from rich.progress import Progress
    RICH_AVAILABLE = True
    console = Console()
except ImportError:
    RICH_AVAILABLE = False
    print("Rich library not found. Using basic console output.")
    print("Install rich for a better experience: pip install rich")
    print()

# Enable debug mode
DEBUG = False

def debug_print(message):
    """Print debug messages if debug mode is enabled."""
    if DEBUG:
        if RICH_AVAILABLE:
            console.print(f"[dim cyan][DEBUG][/] {message}")
        else:
            print(f"[DEBUG] {message}")

def print_color(text, color=None, style=None):
    """Print colored text if rich is available, otherwise print plain text."""
    if RICH_AVAILABLE:
        style_str = f"[{color}]" if color else ""
        style_str += f"[{style}]" if style else ""
        console.print(f"{style_str}{text}[/]" if style_str else text)
    else:
        print(text)

def prompt(text, choices=None, default=None):
    """Prompt the user for input."""
    if RICH_AVAILABLE:
        if choices:
            return Prompt.ask(f"[bold cyan]{text}", choices=choices, default=default)
        else:
            return Prompt.ask(f"[bold cyan]{text}", default=default)
    else:
        if choices:
            choice_str = "/".join(choices)
            result = input(f"{text} ({choice_str}) [{default}]: ") or default
            while result not in choices:
                print(f"Invalid choice. Please select from: {choice_str}")
                result = input(f"{text} ({choice_str}) [{default}]: ") or default
            return result
        else:
            return input(f"{text} [{default}]: ") or default

def confirm(text, default=False):
    """Ask for confirmation."""
    if RICH_AVAILABLE:
        return Confirm.ask(f"[bold cyan]{text}", default=default)
    else:
        default_str = "Y/n" if default else "y/N"
        result = input(f"{text} [{default_str}]: ").lower()
        if not result:
            return default
        return result.startswith('y')

def format_duration(seconds):
    """Format seconds to mm:ss or hh:mm:ss format."""
    if seconds is None:
        return "LIVE"
    
    hours, remainder = divmod(int(seconds), 3600)
    minutes, seconds = divmod(remainder, 60)
    
    if hours:
        return f"{hours}:{minutes:02d}:{seconds:02d}"
    else:
        return f"{minutes:02d}:{seconds:02d}"

def send_message(socket_path, command, request_id=None, params=None):
    """Send a message to the Unix Domain Socket and receive a response."""
    if not os.path.exists(socket_path):
        print_color(f"Socket file not found: {socket_path}", "red", "bold")
        print_color("Make sure the downloader service is running.", "yellow")
        return None
    
    if request_id is None:
        request_id = str(uuid.uuid4())
        
    message = {
        "command": command,
        "id": request_id,
        "timestamp": datetime.now(UTC).isoformat()  # Using UTC timezone
    }
    
    if params:
        message["params"] = params
    
    debug_print(f"Sending message: {json.dumps(message, indent=2)}")
    
    try:
        client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        client.settimeout(60)  # 60 second timeout
        client.connect(socket_path)
        
        # Serialize the message to JSON
        json_data = json.dumps(message).encode('utf-8')
        
        # Create length prefix (4 bytes, big-endian)
        length_prefix = struct.pack('!I', len(json_data))
        
        # Send the message with its length prefix
        client.sendall(length_prefix + json_data)
        
        # Read response
        response_data = b''
        
        # Read header (4 bytes)
        header = b''
        while len(header) < 4:
            chunk = client.recv(4 - len(header))
            if not chunk:
                break
            header += chunk
        
        if len(header) < 4:
            print_color("Incomplete response received", "red", "bold")
            return None
        
        # Unpack response length
        message_length = struct.unpack('!I', header)[0]
        
        # Read the entire message
        while len(response_data) < message_length:
            chunk = client.recv(min(4096, message_length - len(response_data)))
            if not chunk:
                break
            response_data += chunk
        
        # Close the connection
        client.close()
        
        # Parse JSON response
        if response_data:
            try:
                response = json.loads(response_data.decode('utf-8'))
                debug_print(f"Received response: {json.dumps(response, indent=2)}")
                return response
            except json.JSONDecodeError as e:
                print_color(f"Error decoding JSON response: {e}", "red", "bold")
                debug_print(f"Raw response: {response_data.decode('utf-8', errors='replace')}")
                return None
        else:
            print_color("Empty response received", "red", "bold")
            return None
    
    except socket.timeout:
        print_color("Connection timed out", "red", "bold")
        return None
    except ConnectionRefusedError:
        print_color("Connection refused", "red", "bold")
        return None
    except Exception as e:
        print_color(f"Error communicating with socket: {e}", "red", "bold")
        return None

def display_search_results(results):
    """Display search results in a table."""
    if not results:
        print_color("No results found.", "yellow")
        return []
    
    # Handle case where results might be a string instead of a list
    if isinstance(results, str):
        print_color(f"Unexpected results format: {results}", "red", "bold")
        return []
    
    if RICH_AVAILABLE:
        table = Table(show_header=True, header_style="bold magenta")
        table.add_column("#", style="dim", width=4)
        table.add_column("Title", style="cyan")
        table.add_column("Duration", style="green", justify="right")
        table.add_column("Uploader", style="blue")
        table.add_column("Platform", style="yellow")
        
        for i, result in enumerate(results, 1):
            title = result.get('title', 'Unknown')
            duration = format_duration(result.get('duration'))
            uploader = result.get('uploader', 'Unknown')
            platform = result.get('platform', 'Unknown').replace('https://', '')
            
            table.add_row(
                str(i),
                title,
                duration,
                uploader,
                platform
            )
        
        console.print(table)
    else:
        print("-" * 80)
        print(f"{'#':<4} {'Title':<40} {'Duration':<10} {'Uploader':<15} {'Platform':<10}")
        print("-" * 80)
        
        for i, result in enumerate(results, 1):
            title = result.get('title', 'Unknown')
            if len(title) > 40:
                title = title[:37] + "..."
                
            uploader = result.get('uploader', 'Unknown')
            if len(uploader) > 15:
                uploader = uploader[:12] + "..."
                
            platform = result.get('platform', 'Unknown').replace('https://', '')
                
            print(f"{i:<4} {title:<40} {format_duration(result.get('duration')):<10} {uploader:<15} {platform:<10}")
        
        print("-" * 80)
        
    return results

def display_download_result(result):
    """Display download result."""
    if not result:
        print_color("Download failed.", "red", "bold")
        return
    
    # Handle different result formats
    if isinstance(result, str):
        print_color(f"Unexpected result format: {result}", "red", "bold")
        return
    
    print_color("Download successful!", "green", "bold")
    print_color(f"Title: {result.get('title', 'Unknown')}", "cyan", "bold")
    print_color(f"Duration: {format_duration(result.get('duration'))}", "cyan", "bold")
    print_color(f"File: {result.get('filename', 'Unknown')}", "cyan", "bold")
    
    file_size = result.get('file_size')
    if file_size:
        print_color(f"Size: {file_size / (1024*1024):.2f} MB", "cyan", "bold")
    else:
        print_color("Size: Unknown", "cyan", "bold")
        
    print_color(f"Platform: {result.get('platform', 'Unknown')}", "cyan", "bold")
    
    if result.get('skipped'):
        print_color("Note: File was already downloaded.", "yellow")

def display_playlist_result(result):
    """Display playlist download results."""
    if not result:
        print_color("Playlist download failed.", "red", "bold")
        return
    
    # Handle different result formats
    if isinstance(result, str):
        print_color(f"Unexpected result format: {result}", "red", "bold")
        return
    
    print_color("Playlist download successful!", "green", "bold")
    print_color(f"Title: {result.get('playlist_title', 'Unknown')}", "cyan", "bold")
    print_color(f"URL: {result.get('playlist_url', 'Unknown')}", "cyan", "bold")
    print_color(f"Items downloaded: {result.get('count', 0)}", "cyan", "bold")
    
    items = result.get('items', [])
    if items:
        if RICH_AVAILABLE:
            table = Table(show_header=True, header_style="bold magenta")
            table.add_column("#", style="dim", width=4)
            table.add_column("Title", style="cyan")
            table.add_column("Duration", style="green", justify="right")
            table.add_column("Size (MB)", style="blue", justify="right")
            table.add_column("Status", style="yellow")
            
            for i, item in enumerate(items, 1):
                size_mb = "N/A"
                if item.get('file_size'):
                    size_mb = f"{item.get('file_size') / (1024*1024):.2f}"
                    
                status = "[yellow]Skipped[/]" if item.get('skipped') else "[green]Downloaded[/]"
                
                table.add_row(
                    str(i),
                    item.get('title', 'Unknown'),
                    format_duration(item.get('duration')),
                    size_mb,
                    status
                )
            
            console.print(table)
        else:
            print("-" * 80)
            print(f"{'#':<4} {'Title':<40} {'Duration':<10} {'Size (MB)':<12} {'Status':<10}")
            print("-" * 80)
            
            for i, item in enumerate(items, 1):
                title = item.get('title', 'Unknown')
                if len(title) > 40:
                    title = title[:37] + "..."
                    
                size_mb = "N/A"
                if item.get('file_size'):
                    size_mb = f"{item.get('file_size') / (1024*1024):.2f}"
                    
                status = "Skipped" if item.get('skipped') else "Downloaded"
                    
                print(f"{i:<4} {title:<40} {format_duration(item.get('duration')):<10} {size_mb:<12} {status:<10}")
            
            print("-" * 80)

def search_command(args, socket_path):
    """Handle search command."""
    # Initialize args if needed
    if not hasattr(args, 'query'):
        args.query = None
    if not hasattr(args, 'platform'):
        args.platform = None
    if not hasattr(args, 'limit'):
        args.limit = None
    if not hasattr(args, 'max_duration'):
        args.max_duration = None
    if not hasattr(args, 'max_size'):
        args.max_size = None
    if not hasattr(args, 'include_live'):
        args.include_live = None
        
    query = args.query or prompt("Enter search query")
    
    platform_choices = ["youtube", "soundcloud", "ytmusic"]
    platform = args.platform or prompt("Select platform", choices=platform_choices, default="youtube")
    
    limit = args.limit
    if limit is None:
        limit_str = prompt("Number of results", default="5")
        limit = int(limit_str)
    
    include_live = args.include_live
    if include_live is None:
        include_live = confirm("Include live streams?", default=False)
    
    print_color(f"Searching for: {query} on {platform}...", "bold")
    
    params = {
        "query": query,
        "platform": platform,
        "limit": limit,
        "include_live": include_live
    }
    
    response = send_message(socket_path, "search", params=params)
    
    if not response:
        print_color("No response received from service", "red", "bold")
        return
    
    if isinstance(response, str):
        print_color(f"Unexpected response format: {response}", "red", "bold")
        return
    
    if response.get("status") != "success":
        error = response.get("error", "Unknown error")
        print_color(f"Search failed: {error}", "red", "bold")
        return
    
    # Handle the nested results structure
    data = response.get("data", {})
    if isinstance(data, str):
        print_color(f"Unexpected data format: {data}", "red", "bold")
        return
    
    # Check if we have the double-nested results structure
    results = None
    
    if "results" in data:
        results_data = data.get("results")
        if isinstance(results_data, dict) and "results" in results_data:
            # Double-nested structure: data.results.results[]
            results = results_data.get("results", [])
        elif isinstance(results_data, list):
            # Single-nested structure: data.results[]
            results = results_data
    
    # Fallback if we can't find the results
    if results is None:
        print_color("Could not find results in the response. Response structure:", "red", "bold")
        print_color(json.dumps(data, indent=2), "yellow")
        return
    
    if not results:
        print_color("No results found.", "yellow")
        return
    
    results_list = display_search_results(results)
    
    if results_list and confirm("Download a result?", default=False):
        choice_str = prompt(
            "Enter number to download",
            choices=[str(i) for i in range(1, len(results_list) + 1)],
            default="1"
        )
        choice = int(choice_str)
        
        selected = results_list[choice - 1]
        print_color(f"Downloading: {selected.get('title', 'Unknown')}...", "bold")
        
        max_duration = args.max_duration
        if max_duration is None:
            max_duration_str = prompt("Maximum duration in seconds (0 for unlimited)", default="0")
            max_duration = int(max_duration_str)
            if max_duration == 0:
                max_duration = None
        
        max_size = args.max_size
        if max_size is None:
            max_size_str = prompt("Maximum size in MB (0 for unlimited)", default="0")
            max_size = int(max_size_str)
            if max_size == 0:
                max_size = None
        
        download_params = {
            "url": selected.get('url'),
            "max_duration_seconds": max_duration,
            "max_size_mb": max_size,
            "allow_live": include_live
        }
        
        download_response = send_message(socket_path, "download_audio", params=download_params)
        
        if not download_response:
            print_color("No response received", "red", "bold")
            return
        
        if isinstance(download_response, str):
            print_color(f"Unexpected response format: {download_response}", "red", "bold")
            return
        
        if download_response.get("status") != "success":
            error = download_response.get("error", "Unknown error")
            print_color(f"Download failed: {error}", "red", "bold")
            return
        
        display_download_result(download_response.get("data"))

def download_command(args, socket_path):
    """Handle download command."""
    # Initialize args if needed
    if not hasattr(args, 'url'):
        args.url = None
    if not hasattr(args, 'max_duration'):
        args.max_duration = None
    if not hasattr(args, 'max_size'):
        args.max_size = None
    if not hasattr(args, 'include_live'):
        args.include_live = None
        
    url = args.url or prompt("Enter URL to download")
    
    max_duration = args.max_duration
    if max_duration is None:
        max_duration_str = prompt("Maximum duration in seconds (0 for unlimited)", default="0")
        max_duration = int(max_duration_str)
        if max_duration == 0:
            max_duration = None
    
    max_size = args.max_size
    if max_size is None:
        max_size_str = prompt("Maximum size in MB (0 for unlimited)", default="0")
        max_size = int(max_size_str)
        if max_size == 0:
            max_size = None
    
    allow_live = args.include_live
    if allow_live is None:
        allow_live = confirm("Allow live streams?", default=False)
        
    print_color(f"Downloading: {url}...", "bold")
    
    params = {
        "url": url,
        "max_duration_seconds": max_duration,
        "max_size_mb": max_size,
        "allow_live": allow_live
    }
    
    response = send_message(socket_path, "download_audio", params=params)
    
    if not response:
        print_color("No response received", "red", "bold")
        return
        
    if isinstance(response, str):
        print_color(f"Unexpected response format: {response}", "red", "bold")
        return
        
    if response.get("status") != "success":
        error = response.get("error", "Unknown error")
        print_color(f"Download failed: {error}", "red", "bold")
        return
    
    display_download_result(response.get("data"))

def playlist_command(args, socket_path):
    """Handle playlist download command."""
    # Initialize args if needed
    if not hasattr(args, 'url'):
        args.url = None
    if not hasattr(args, 'max_items'):
        args.max_items = None
    if not hasattr(args, 'max_duration'):
        args.max_duration = None
    if not hasattr(args, 'max_size'):
        args.max_size = None
    if not hasattr(args, 'include_live'):
        args.include_live = None
        
    url = args.url or prompt("Enter playlist URL")
    
    max_items = args.max_items
    if max_items is None:
        max_items_str = prompt("Maximum number of items (0 for unlimited)", default="5")
        max_items = int(max_items_str)
        if max_items == 0:
            max_items = None
    
    max_duration = args.max_duration
    if max_duration is None:
        max_duration_str = prompt("Maximum duration per item in seconds (0 for unlimited)", default="0")
        max_duration = int(max_duration_str)
        if max_duration == 0:
            max_duration = None
    
    max_size = args.max_size
    if max_size is None:
        max_size_str = prompt("Maximum size per item in MB (0 for unlimited)", default="0")
        max_size = int(max_size_str)
        if max_size == 0:
            max_size = None
    
    allow_live = args.include_live
    if allow_live is None:
        allow_live = confirm("Allow live streams?", default=False)
        
    print_color(f"Downloading playlist: {url}...", "bold")
    
    params = {
        "url": url,
        "max_items": max_items,
        "max_duration_seconds": max_duration,
        "max_size_mb": max_size,
        "allow_live": allow_live
    }
    
    response = send_message(socket_path, "download_playlist", params=params)
    
    if not response:
        print_color("No response received", "red", "bold")
        return
        
    if isinstance(response, str):
        print_color(f"Unexpected response format: {response}", "red", "bold")
        return
        
    if response.get("status") != "success":
        error = response.get("error", "Unknown error")
        print_color(f"Download failed: {error}", "red", "bold")
        return
    
    display_playlist_result(response.get("data"))

def ping_command(socket_path):
    """Send a ping command to check if the service is responsive."""
    timestamp = datetime.now(UTC).isoformat()  # Using UTC timezone
    
    params = {
        "timestamp": timestamp
    }
    
    print_color("Pinging downloader service...", "cyan")
    
    response = send_message(socket_path, "ping", params=params)
    
    if not response:
        print_color("No response received from service", "red", "bold")
        return False
    
    if isinstance(response, str):
        print_color(f"Unexpected response format: {response}", "red", "bold")
        return False
        
    if response.get("status") != "success":
        error = response.get("error", "Unknown error")
        print_color(f"Error: {error}", "red", "bold")
        return False
    
    data = response.get("data", {})
    print_color("Service is responsive!", "green", "bold")
    print_color(f"Response: {data.get('message', 'Unknown')}", "cyan")
    print_color(f"Echo timestamp: {data.get('timestamp', 'None')}", "cyan")
    
    # Calculate round trip time if timestamps are available
    if timestamp and response.get("timestamp"):
        try:
            sent = datetime.fromisoformat(timestamp)
            received = datetime.fromisoformat(response.get("timestamp"))
            rtt = (received - sent).total_seconds() * 1000
            print_color(f"Round trip time: {rtt:.2f} ms", "cyan")
        except (ValueError, TypeError):
            pass
    
    return True

def main():
    parser = argparse.ArgumentParser(
        description="Client for interacting with the downloader service",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  ./test.py search --query "lo-fi beats" --platform youtube
  ./test.py download --url "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
  ./test.py playlist --url "https://www.youtube.com/playlist?list=PLFgquLnL59alCl_2TQvOiD5Vgm1hCaGSI"
  ./test.py ping
"""
    )
    
    # Global arguments
    parser.add_argument("--socket", help="Path to Unix Domain Socket", default="/tmp/downloader.sock")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    parser.add_argument("--debug", "-d", action="store_true", help="Enable debug output")
    
    subparsers = parser.add_subparsers(dest="command", help="Commands")
    
    # Ping command
    ping_parser = subparsers.add_parser("ping", help="Check if service is responsive")
    
    # Search command
    search_parser = subparsers.add_parser("search", help="Search for audio")
    search_parser.add_argument("--query", help="Search query")
    search_parser.add_argument("--platform", choices=["youtube", "soundcloud", "ytmusic"], 
                              help="Platform to search on")
    search_parser.add_argument("--limit", type=int, help="Maximum number of results")
    search_parser.add_argument("--max-duration", type=int, help="Maximum duration in seconds")
    search_parser.add_argument("--max-size", type=int, help="Maximum size in MB")
    search_parser.add_argument("--include-live", action="store_true", help="Include live streams")
    
    # Download command
    download_parser = subparsers.add_parser("download", help="Download audio from URL")
    download_parser.add_argument("--url", help="URL to download")
    download_parser.add_argument("--max-duration", type=int, help="Maximum duration in seconds")
    download_parser.add_argument("--max-size", type=int, help="Maximum size in MB")
    download_parser.add_argument("--include-live", action="store_true", help="Allow live streams")
    
    # Playlist command
    playlist_parser = subparsers.add_parser("playlist", help="Download playlist")
    playlist_parser.add_argument("--url", help="Playlist URL")
    playlist_parser.add_argument("--max-items", type=int, help="Maximum number of items")
    playlist_parser.add_argument("--max-duration", type=int, help="Maximum duration per item in seconds")
    playlist_parser.add_argument("--max-size", type=int, help="Maximum size per item in MB")
    playlist_parser.add_argument("--include-live", action="store_true", help="Allow live streams")
    
    args = parser.parse_args()
    
    # Enable debug mode if requested
    global DEBUG
    DEBUG = args.debug or args.verbose
    
    socket_path = args.socket
    
    if args.command is None:
        # Interactive mode if no command specified
        if RICH_AVAILABLE:
            console.print("[bold cyan]Downloader Client[/]")
            console.print("[bold cyan]=================[/]")
            console.print()
        else:
            print("Downloader Client")
            print("=================")
            print()
        
        # Check if service is responsive
        if not ping_command(socket_path):
            if confirm("Service appears to be down. Continue anyway?", default=False):
                pass
            else:
                return
        
        choices = {
            "search": "Search for audio",
            "download": "Download audio from URL",
            "playlist": "Download playlist",
            "ping": "Ping the service",
            "exit": "Exit the program"
        }
        
        while True:
            choice_options = list(choices.keys())
            print()
            for i, (cmd, desc) in enumerate(choices.items(), 1):
                print_color(f"{i}. {desc} ({cmd})")
            print()
            
            choice_input = prompt(
                "Choose an option (1-5)",
                choices=["1", "2", "3", "4", "5"],
                default="1"
            )
            
            choice = choice_options[int(choice_input) - 1]
            
            if choice == "exit":
                break
            elif choice == "search":
                search_command(argparse.Namespace(), socket_path)
            elif choice == "download":
                download_command(argparse.Namespace(), socket_path)
            elif choice == "playlist":
                playlist_command(argparse.Namespace(), socket_path)
            elif choice == "ping":
                ping_command(socket_path)
    else:
        # Command-line mode
        if args.command == "search":
            search_command(args, socket_path)
        elif args.command == "download":
            download_command(args, socket_path)
        elif args.command == "playlist":
            playlist_command(args, socket_path)
        elif args.command == "ping":
            ping_command(socket_path)

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print_color("\nOperation cancelled by user.", "red", "bold")
        sys.exit(0)
    except Exception as e:
        print_color(f"Error: {e}", "red", "bold")
        if '-v' in sys.argv or '--verbose' in sys.argv:
            import traceback
            traceback.print_exc()
        sys.exit(1)
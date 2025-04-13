import socket
import json
import struct
import uuid
import sys

def send_request(socket_path, command, params=None):
    request_id = str(uuid.uuid4())
    
    request = {
        "command": command,
        "id": request_id,
        "params": params or {}
    }
    
    client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    
    try:
        client.connect(socket_path)
        
        json_data = json.dumps(request).encode('utf-8')
        length_prefix = struct.pack('!I', len(json_data))
        client.sendall(length_prefix + json_data)
        
        header = b''
        while len(header) < 4:
            chunk = client.recv(4 - len(header))
            if not chunk:
                return None
            header += chunk
        
        message_length = struct.unpack('!I', header)[0]
        
        message = b''
        while len(message) < message_length:
            chunk = client.recv(min(4096, message_length - len(message)))
            if not chunk:
                return None
            message += chunk
        
        response = json.loads(message.decode('utf-8'))
        return response
    except Exception as e:
        print(f"Error communicating with UDS server: {e}")
        return None
    finally:
        client.close()

def print_json(data):
    print(json.dumps(data, indent=2))

if __name__ == "__main__":
    socket_path = "/tmp/downloader.sock"
    
    if len(sys.argv) < 2:
        print("Usage: python uds_client.py <command> [params]")
        print("Available commands: ping, download_audio, search")
        sys.exit(1)
    
    command = sys.argv[1]
    
    if command == "ping":
        response = send_request(socket_path, "ping", {"timestamp": "now"})
        print_json(response)
    
    elif command == "download_audio":
        if len(sys.argv) < 3:
            print("Usage: python uds_client.py download_audio <youtube_url>")
            sys.exit(1)
        
        url = sys.argv[2]
        params = {
            "url": url,
            "max_duration_seconds": 600,
            "max_size_mb": 50
        }
        
        print(f"Downloading audio from: {url}")
        response = send_request(socket_path, "download_audio", params)
        print_json(response)
    
    elif command == "search":
        if len(sys.argv) < 3:
            print("Usage: python uds_client.py search <query>")
            sys.exit(1)
        
        query = sys.argv[2]
        limit = 3
        if len(sys.argv) > 3:
            try:
                limit = int(sys.argv[3])
            except ValueError:
                pass
        
        params = {
            "query": query,
            "platform": "youtube",
            "limit": limit
        }
        
        print(f"Searching for: {query} (limit: {limit})")
        response = send_request(socket_path, "search", params)
        print_json(response)
    
    else:
        print(f"Unknown command: {command}")
        print("Available commands: ping, download_audio, search")
        sys.exit(1)
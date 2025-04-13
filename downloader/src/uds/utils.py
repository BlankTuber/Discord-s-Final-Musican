import os
import json
import socket
import struct

_config = {}

def init(cfg):
    global _config
    _config.update(cfg)
    print("UDS utils module initialized")

def ensure_socket_dir_exists(socket_path):
    socket_dir = os.path.dirname(socket_path)
    os.makedirs(socket_dir, exist_ok=True)

def cleanup_socket(socket_path):
    if os.path.exists(socket_path):
        os.unlink(socket_path)

def read_json_message(conn):
    try:
        header = b''
        while len(header) < 4:
            chunk = conn.recv(4 - len(header))
            if not chunk:
                return None
            header += chunk
        
        message_length = struct.unpack('!I', header)[0]
        
        message = b''
        while len(message) < message_length:
            chunk = conn.recv(min(4096, message_length - len(message)))
            if not chunk:
                return None
            message += chunk
        
        return message.decode('utf-8')
    except Exception as e:
        print(f"Error reading from socket: {e}")
        return None

def send_json_message(conn, data):
    try:
        json_data = json.dumps(data)
        message = json_data.encode('utf-8')
        
        length_prefix = struct.pack('!I', len(message))
        
        conn.sendall(length_prefix + message)
        return True
    except Exception as e:
        print(f"Error sending to socket: {e}")
        return False
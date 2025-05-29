import os
import json
import socket
import struct
import time

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
        original_timeout = conn.gettimeout()
        conn.settimeout(120.0)  # 2 minute timeout for reading
        
        start_time = time.time()
        header = b''
        while len(header) < 4:
            try:
                chunk = conn.recv(4 - len(header))
                if not chunk:
                    print("UDS Utils: Connection closed while reading header")
                    return None
                header += chunk
            except socket.timeout:
                elapsed = time.time() - start_time
                print(f"UDS Utils: Timeout reading header after {elapsed:.2f} seconds")
                return None
            except ConnectionResetError:
                print("UDS Utils: Connection reset by peer")
                return None
        
        message_length = struct.unpack('!I', header)[0]
        print(f"UDS Utils: Message length: {message_length} bytes")
        
        if message_length > 100 * 1024 * 1024:
            print(f"UDS Utils: Message length too large: {message_length}")
            return None
        
        if message_length == 0:
            print("UDS Utils: Zero-length message received")
            return None
        
        message = b''
        read_start = time.time()
        bytes_read = 0
        
        while len(message) < message_length:
            try:
                chunk_size = min(8192, message_length - len(message))
                chunk = conn.recv(chunk_size)
                if not chunk:
                    elapsed = time.time() - start_time
                    print(f"UDS Utils: Connection closed while reading message body after {elapsed:.2f} seconds")
                    return None
                message += chunk
                bytes_read += len(chunk)
                
                # Reset timeout on progress to handle large messages
                if bytes_read % (64 * 1024) == 0:  # Every 64KB
                    conn.settimeout(120.0)
                
                if message_length > 1024*1024 and len(message) % (1024*1024) < 8192:
                    print(f"UDS Utils: Read {len(message)/1024/1024:.1f}MB of {message_length/1024/1024:.1f}MB")
            except socket.timeout:
                elapsed = time.time() - start_time
                print(f"UDS Utils: Timeout reading message body after {elapsed:.2f} seconds, read {len(message)} of {message_length} bytes")
                return None
            except ConnectionResetError:
                print("UDS Utils: Connection reset by peer while reading body")
                return None
        
        read_time = time.time() - read_start
        print(f"UDS Utils: Read complete message of {len(message)} bytes in {read_time:.2f} seconds")
        
        # Restore original timeout
        if original_timeout is not None:
            conn.settimeout(original_timeout)
        
        try:
            decoded = message.decode('utf-8')
            json.loads(decoded)  # Validate JSON
            return decoded
        except json.JSONDecodeError as e:
            print(f"UDS Utils: Invalid JSON received: {e}")
            # Log first 500 chars for debugging
            preview = decoded[:500] if len(decoded) > 500 else decoded
            print(f"UDS Utils: JSON preview: {repr(preview)}")
            return None
        except UnicodeDecodeError as e:
            print(f"UDS Utils: Unicode decode error: {e}")
            return None
            
    except Exception as e:
        print(f"UDS Utils: Error reading from socket: {e}")
        import traceback
        print(f"UDS Utils: {traceback.format_exc()}")
        return None

def send_json_message(conn, data):
    try:
        original_timeout = conn.gettimeout()
        conn.settimeout(120.0)  # 2 minute timeout for sending
        
        start_time = time.time()
        json_data = json.dumps(data)
        message = json_data.encode('utf-8')
        
        length_prefix = struct.pack('!I', len(message))
        
        print(f"UDS Utils: Sending message of {len(message)} bytes")
        
        try:
            conn.sendall(length_prefix)
        except (ConnectionResetError, BrokenPipeError) as e:
            print(f"UDS Utils: Connection error while sending header: {e}")
            return False
        except socket.timeout as e:
            print(f"UDS Utils: Timeout while sending header: {e}")
            return False
        
        chunk_size = 8192
        sent = 0
        while sent < len(message):
            try:
                chunk = message[sent:sent+chunk_size]
                conn.sendall(chunk)
                sent += len(chunk)
                
                # Reset timeout on progress for large messages
                if sent % (64 * 1024) == 0:  # Every 64KB
                    conn.settimeout(120.0)
                
                if len(message) > 1024*1024 and sent % (1024*1024) < chunk_size:
                    print(f"UDS Utils: Sent {sent/1024/1024:.1f}MB of {len(message)/1024/1024:.1f}MB")
            except (ConnectionResetError, BrokenPipeError) as e:
                print(f"UDS Utils: Connection error while sending body: {e}")
                return False
            except socket.timeout as e:
                elapsed = time.time() - start_time
                print(f"UDS Utils: Timeout while sending body after {elapsed:.2f}s, sent {sent} of {len(message)} bytes: {e}")
                return False
        
        elapsed = time.time() - start_time
        print(f"UDS Utils: Message sent successfully in {elapsed:.2f} seconds")
        
        # Restore original timeout
        if original_timeout is not None:
            conn.settimeout(original_timeout)
        
        return True
    except Exception as e:
        print(f"UDS Utils: Error sending to socket: {e}")
        import traceback
        print(f"UDS Utils: {traceback.format_exc()}")
        return False
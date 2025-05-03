import socket
import os
import threading
import time
import traceback
from uds import handlers, utils, protocol

_socket = None
_thread = None
_running = False
_config = {}

def init(cfg):
    global _config
    _config.update(cfg)
    print("UDS server module initialized")

def start(socket_path, allowed_origins):
    global _socket, _thread, _running
    
    if _running:
        print("Server already running")
        return False
    
    try:
        _socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        _socket.bind(socket_path)
        _socket.listen(5)
        
        _running = True
        _thread = threading.Thread(target=_server_loop, daemon=True)
        _thread.start()
        
        print(f"UDS server started on {socket_path}")
        return True
    except Exception as e:
        print(f"Failed to start UDS server: {e}")
        if _socket:
            _socket.close()
            _socket = None
        return False

def stop():
    global _socket, _thread, _running
    
    if not _running:
        return False
    
    try:
        _running = False
        
        if _socket:
            _socket.close()
            _socket = None
        
        if _thread and _thread.is_alive():
            _thread.join(timeout=3.0)
        
        print("UDS server stopped")
        return True
    except Exception as e:
        print(f"Error stopping UDS server: {e}")
        return False

def _server_loop():
    global _socket, _running
    
    print("UDS server loop started")
    
    while _running and _socket:
        try:
            _socket.settimeout(1.0)
            client, _ = _socket.accept()
            _socket.settimeout(None)
            
            print(f"UDS: New client connection accepted")
            
            client_thread = threading.Thread(
                target=_handle_client,
                args=(client,),
                daemon=True
            )
            client_thread.start()
        except socket.timeout:
            continue
        except ConnectionAbortedError:
            break
        except Exception as e:
            if _running:
                print(f"Error in server loop: {e}")
                print(f"Traceback: {traceback.format_exc()}")
            break
    
    print("Server loop terminated")

def _handle_client(client_socket):
    start_time = time.time()
    try:
        client_socket.settimeout(300)  # 5 minute timeout for client operations
        print(f"UDS: Reading request from client...")
        
        data = utils.read_json_message(client_socket)
        if not data:
            print("UDS: No data received from client")
            return
        
        print(f"UDS: Received data of length {len(data)}")
        
        request = protocol.parse_request(data)
        if not request:
            print("UDS: Invalid request format")
            error_response = protocol.create_error_response("Invalid request format")
            utils.send_json_message(client_socket, error_response)
            return
        
        command = request.get("command", "unknown")
        request_id = request.get("id", "unknown")
        print(f"UDS: Handling request - Command: {command}, ID: {request_id}")
        
        response = handlers.process_request(request, _config)
        
        print(f"UDS: Sending response for {command}, ID: {request_id}")
        utils.send_json_message(client_socket, response)
        elapsed = time.time() - start_time
        print(f"UDS: Request handled in {elapsed:.2f} seconds - Command: {command}, ID: {request_id}")
        
    except Exception as e:
        elapsed = time.time() - start_time
        print(f"UDS: Error handling client after {elapsed:.2f} seconds: {e}")
        print(f"UDS: Traceback: {traceback.format_exc()}")
        try:
            error_response = protocol.create_error_response(f"Server error: {str(e)}")
            utils.send_json_message(client_socket, error_response)
        except Exception as e2:
            print(f"UDS: Failed to send error response: {e2}")
    finally:
        client_socket.close()
        print("UDS: Client connection closed")
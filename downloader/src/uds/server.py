import socket
import os
import threading
import time
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
    
    while _running and _socket:
        try:
            _socket.settimeout(1.0)
            client, _ = _socket.accept()
            _socket.settimeout(None)
            
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
            break
    
    print("Server loop terminated")

def _handle_client(client_socket):
    try:
        data = utils.read_json_message(client_socket)
        if not data:
            return
        
        request = protocol.parse_request(data)
        if not request:
            error_response = protocol.create_error_response("Invalid request format")
            utils.send_json_message(client_socket, error_response)
            return
        
        response = handlers.process_request(request, _config)
        
        utils.send_json_message(client_socket, response)
    except Exception as e:
        print(f"Error handling client: {e}")
        try:
            error_response = protocol.create_error_response(f"Server error: {str(e)}")
            utils.send_json_message(client_socket, error_response)
        except:
            pass
    finally:
        client_socket.close()
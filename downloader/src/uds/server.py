import socket
import os
import threading
import time
import traceback
from uds import handlers, utils, protocol
import logger

_socket = None
_thread = None
_running = False
_config = {}

def init(cfg):
    global _config
    _config.update(cfg)
    logger.logger.info("UDS server module initialized")

def start(socket_path, allowed_origins):
    global _socket, _thread, _running
    
    if _running:
        logger.logger.warning("Server already running")
        return False
    
    try:
        _socket = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        _socket.bind(socket_path)
        _socket.listen(5)
        
        _running = True
        _thread = threading.Thread(target=_server_loop, daemon=True)
        _thread.start()
        
        logger.logger.info(f"UDS server started on {socket_path}")
        return True
    except Exception as e:
        logger.logger.error(f"Failed to start UDS server: {e}")
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
        
        logger.logger.info("UDS server stopped")
        return True
    except Exception as e:
        logger.logger.error(f"Error stopping UDS server: {e}")
        return False

def _server_loop():
    global _socket, _running
    
    logger.logger.info("UDS server loop started")
    
    while _running and _socket:
        try:
            _socket.settimeout(1.0)
            client, _ = _socket.accept()
            _socket.settimeout(None)
            
            logger.logger.info("New client connection accepted")
            
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
                logger.logger.error(f"Error in server loop: {e}")
                logger.logger.debug(f"Traceback: {traceback.format_exc()}")
            break
    
    logger.logger.info("Server loop terminated")

def _handle_client(client_socket):
    start_time = time.time()
    try:
        client_socket.settimeout(300)  # 5 minute timeout for client operations
        logger.logger.info("Reading request from client...")
        
        data = utils.read_json_message(client_socket)
        if not data:
            logger.logger.warning("No data received from client")
            return
        
        logger.logger.info(f"Received data of length {len(data)}")
        
        request = protocol.parse_request(data)
        if not request:
            logger.logger.warning("Invalid request format")
            error_response = protocol.create_error_response("Invalid request format")
            utils.send_json_message(client_socket, error_response)
            return
        
        command = request.get("command", "unknown")
        request_id = request.get("id", "unknown")
        logger.logger.info(f"Handling request - Command: {command}, ID: {request_id}")
        
        response = handlers.process_request(request, _config)
        
        logger.logger.info(f"Sending response for {command}, ID: {request_id}")
        utils.send_json_message(client_socket, response)
        elapsed = time.time() - start_time
        logger.logger.info(f"Request handled in {elapsed:.2f} seconds - Command: {command}, ID: {request_id}")
        
    except Exception as e:
        elapsed = time.time() - start_time
        logger.logger.error(f"Error handling client after {elapsed:.2f} seconds: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
        try:
            error_response = protocol.create_error_response(f"Server error: {str(e)}")
            utils.send_json_message(client_socket, error_response)
        except Exception as e2:
            logger.logger.error(f"Failed to send error response: {e2}")
    finally:
        client_socket.close()
        logger.logger.info("Client connection closed")

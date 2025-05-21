import socket
import os
import threading
import time
import traceback
import uuid
from uds import handlers, utils, protocol
import logger

_socket = None
_thread = None
_running = False
_config = {}
_clients = {}  # Dictionary to track active client connections

def init(cfg):
    global _config
    _config.update(cfg)
    
    # Register as an event listener to forward events to clients
    handlers.register_event_listener(handle_event)
    
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
    global _socket, _thread, _running, _clients
    
    if not _running:
        return False
    
    try:
        _running = False
        
        # Close all client connections
        for client_id, client_data in list(_clients.items()):
            try:
                if client_data['socket']:
                    client_data['socket'].close()
            except Exception as e:
                logger.logger.error(f"Error closing client connection {client_id}: {e}")
        
        _clients.clear()
        
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
            
            # Generate a unique client ID
            client_id = str(uuid.uuid4())
            
            logger.logger.info(f"New client connection accepted (ID: {client_id})")
            
            # Store client connection
            _clients[client_id] = {
                'socket': client,
                'connected': True,
                'last_activity': time.time()
            }
            
            client_thread = threading.Thread(
                target=_handle_client,
                args=(client, client_id),
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

def _handle_client(client_socket, client_id):
    global _clients
    
    try:
        client_socket.settimeout(300)  # 5 minute timeout for client operations
        logger.logger.info(f"Starting client handler for {client_id}")
        
        while _running and client_id in _clients and _clients[client_id]['connected']:
            try:
                logger.logger.info(f"Reading request from client {client_id}...")
                
                data = utils.read_json_message(client_socket)
                if not data:
                    logger.logger.warning(f"No data received from client {client_id}, closing connection")
                    break
                
                logger.logger.info(f"Received data of length {len(data)} from client {client_id}")
                
                # Update activity timestamp
                _clients[client_id]['last_activity'] = time.time()
                
                request = protocol.parse_request(data)
                if not request:
                    logger.logger.warning(f"Invalid request format from client {client_id}")
                    error_response = protocol.create_error_response("Invalid request format")
                    utils.send_json_message(client_socket, error_response)
                    continue
                
                command = request.get("command", "unknown")
                request_id = request.get("id", "unknown")
                logger.logger.info(f"Handling request from client {client_id} - Command: {command}, ID: {request_id}")
                
                response = handlers.process_request(request, _config)
                
                logger.logger.info(f"Sending response for {command}, ID: {request_id} to client {client_id}")
                utils.send_json_message(client_socket, response)
                logger.logger.info(f"Request handled - Command: {command}, ID: {request_id}, Client: {client_id}")
                
            except socket.timeout:
                logger.logger.warning(f"Client {client_id} socket timed out waiting for data")
                break
            except ConnectionResetError:
                logger.logger.warning(f"Client {client_id} connection reset")
                break
            except Exception as e:
                logger.logger.error(f"Error handling client {client_id}: {e}")
                logger.logger.debug(f"Traceback: {traceback.format_exc()}")
                try:
                    error_response = protocol.create_error_response(f"Server error: {str(e)}")
                    utils.send_json_message(client_socket, error_response)
                except Exception as e2:
                    logger.logger.error(f"Failed to send error response to client {client_id}: {e2}")
                break
    except Exception as e:
        logger.logger.error(f"Client {client_id} handler exception: {e}")
        logger.logger.debug(f"Traceback: {traceback.format_exc()}")
    finally:
        try:
            client_socket.close()
        except:
            pass
        
        # Remove client from active clients
        if client_id in _clients:
            del _clients[client_id]
        
        logger.logger.info(f"Client {client_id} connection closed")

def handle_event(event_type, event_data):
    """Handle events from the ytdlp_handler and forward to connected clients"""
    global _clients
    
    logger.logger.info(f"Received event: {event_type}")
    
    # Create an event message
    event_message = protocol.create_event_message(event_type, event_data)
    
    # Send to all connected clients
    for client_id, client_data in list(_clients.items()):
        try:
            if client_data['connected']:
                logger.logger.info(f"Sending event {event_type} to client {client_id}")
                utils.send_json_message(client_data['socket'], event_message)
            else:
                logger.logger.debug(f"Skipping disconnected client {client_id}")
        except Exception as e:
            logger.logger.error(f"Error sending event to client {client_id}: {e}")
            # Mark client as disconnected
            client_data['connected'] = False
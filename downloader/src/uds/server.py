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
_clients = {}

def init(cfg):
    global _config
    _config.update(cfg)
    
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
            
            client_id = str(uuid.uuid4())
            
            logger.logger.info(f"New client connection accepted (ID: {client_id})")
            
            _clients[client_id] = {
                'socket': client,
                'connected': True,
                'last_activity': time.time(),
                'last_keepalive': time.time()
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
        client_socket.settimeout(600)  # 10 minute initial timeout
        logger.logger.info(f"Starting client handler for {client_id}")
        
        while _running and client_id in _clients and _clients[client_id]['connected']:
            try:
                logger.logger.debug(f"Reading request from client {client_id}...")
                
                data = utils.read_json_message(client_socket)
                if not data:
                    logger.logger.warning(f"No data received from client {client_id}, closing connection")
                    break
                
                logger.logger.debug(f"Received data of length {len(data)} from client {client_id}")
                
                current_time = time.time()
                _clients[client_id]['last_activity'] = current_time
                
                request = protocol.parse_request(data)
                if not request:
                    logger.logger.warning(f"Invalid request format from client {client_id}")
                    error_response = protocol.create_error_response("Invalid request format")
                    utils.send_json_message(client_socket, error_response)
                    continue
                
                command = request.get("command", "unknown")
                request_id = request.get("id", "unknown")
                
                # Handle keepalive pings specially
                if command == "ping":
                    params = request.get("params", {})
                    if params.get("keepalive"):
                        logger.logger.debug(f"Handling keepalive ping from client {client_id}")
                        _clients[client_id]['last_keepalive'] = current_time
                        
                        # Reset socket timeout for active connections
                        client_socket.settimeout(600)  # Reset to 10 minutes
                        
                        # Send immediate pong response
                        keepalive_response = protocol.create_success_response(request_id, {
                            "message": "pong",
                            "timestamp": params.get("timestamp", "none"),
                            "server_time": time.time(),
                            "keepalive": True
                        })
                        utils.send_json_message(client_socket, keepalive_response)
                        continue
                
                logger.logger.info(f"Handling request from client {client_id} - Command: {command}, ID: {request_id}")
                
                response = handlers.process_request(request, _config)
                
                logger.logger.info(f"Sending response for {command}, ID: {request_id} to client {client_id}")
                utils.send_json_message(client_socket, response)
                logger.logger.info(f"Request handled - Command: {command}, ID: {request_id}, Client: {client_id}")
                
                # Check if we should adjust timeout based on recent activity
                time_since_keepalive = current_time - _clients[client_id]['last_keepalive']
                if time_since_keepalive < 300:  # If keepalive within 5 minutes, extend timeout
                    client_socket.settimeout(600)  # 10 minutes
                else:
                    client_socket.settimeout(300)  # 5 minutes for inactive connections
                
            except socket.timeout:
                current_time = time.time()
                time_since_activity = current_time - _clients[client_id]['last_activity']
                time_since_keepalive = current_time - _clients[client_id]['last_keepalive']
                
                # If we haven't seen keepalive in a while, close connection
                if time_since_keepalive > 600:  # 10 minutes without keepalive
                    logger.logger.warning(f"Client {client_id} timeout - no keepalive in {time_since_keepalive:.0f} seconds")
                    break
                elif time_since_activity > 300:  # 5 minutes without any activity
                    logger.logger.warning(f"Client {client_id} timeout - no activity in {time_since_activity:.0f} seconds")
                    break
                else:
                    logger.logger.debug(f"Client {client_id} socket timeout, but recent activity detected, continuing...")
                    client_socket.settimeout(120)  # Shorter timeout to check more frequently
                    continue
                    
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
        
        if client_id in _clients:
            del _clients[client_id]
        
        logger.logger.info(f"Client {client_id} connection closed")

def handle_event(event_type, event_data):
    global _clients
    
    logger.logger.info(f"Received event: {event_type}")
    
    event_message = protocol.create_event_message(event_type, event_data)
    
    for client_id, client_data in list(_clients.items()):
        try:
            if client_data['connected']:
                logger.logger.debug(f"Sending event {event_type} to client {client_id}")
                utils.send_json_message(client_data['socket'], event_message)
            else:
                logger.logger.debug(f"Skipping disconnected client {client_id}")
        except Exception as e:
            logger.logger.error(f"Error sending event to client {client_id}: {e}")
            client_data['connected'] = False
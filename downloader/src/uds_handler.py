import os
from uds import server, protocol, handlers, utils

config = {}
_server_running = False

def initialize(cfg):
    global config
    config.update(cfg)
    
    server.init(config)
    protocol.init(config)
    handlers.init(config)
    utils.init(config)
    
    socket_dir = os.path.dirname(config["uds_link"])
    os.makedirs(socket_dir, exist_ok=True)
    
    if os.path.exists(config["uds_link"]):
        os.unlink(config["uds_link"])
    
    print(f"UDS handler initialized with socket at: {config['uds_link']}")
    return True

def start_server():
    global _server_running
    
    if _server_running:
        print("UDS server is already running")
        return False
    
    success = server.start(
        socket_path=config["uds_link"],
        allowed_origins=config["allowed_origins"]
    )
    
    _server_running = success
    return success

def stop_server():
    global _server_running
    
    if not _server_running:
        print("UDS server is not running")
        return False
    
    success = server.stop()
    if success:
        _server_running = False
        if os.path.exists(config["uds_link"]):
            os.unlink(config["uds_link"])
    
    return success

def is_running():
    return _server_running

def register_command_handler(command, handler_func):
    return handlers.register_handler(command, handler_func)
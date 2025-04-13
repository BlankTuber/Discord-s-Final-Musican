import json
import uuid
from datetime import datetime

_config = {}

def init(cfg):
    global _config
    _config.update(cfg)
    print("UDS protocol module initialized")

REQUEST_SCHEMA = {
    "type": "object",
    "required": ["command", "id"],
    "properties": {
        "command": {"type": "string"},
        "id": {"type": "string"},
        "params": {"type": "object"},
        "timestamp": {"type": "string"}
    }
}

RESPONSE_SCHEMA = {
    "type": "object",
    "required": ["status", "id"],
    "properties": {
        "status": {"type": "string", "enum": ["success", "error"]},
        "id": {"type": "string"},
        "data": {"type": "object"},
        "error": {"type": "string"},
        "timestamp": {"type": "string"}
    }
}

def validate_request(request):
    if not isinstance(request, dict):
        return False
    
    if "command" not in request or not isinstance(request["command"], str):
        return False
        
    if "id" not in request or not isinstance(request["id"], str):
        return False
    
    return True

def parse_request(data):
    try:
        request = json.loads(data)
        if not validate_request(request):
            return None
        return request
    except json.JSONDecodeError:
        return None

def create_success_response(request_id, data=None):
    response = {
        "status": "success",
        "id": request_id,
        "timestamp": datetime.utcnow().isoformat()
    }
    
    if data is not None:
        response["data"] = data
        
    return response

def create_error_response(error_message, request_id=None):
    if request_id is None:
        request_id = str(uuid.uuid4())
        
    return {
        "status": "error",
        "id": request_id,
        "error": error_message,
        "timestamp": datetime.utcnow().isoformat()
    }
# audience.py

import socket
import json

DEFAULT_SOCKET_PATH="/var/spool/orchestra/conductor.sock"

class ServerError(Exception):
    pass

def submit_job(score, scope, target, args=None, sockname=DEFAULT_SOCKET_PATH):
    reqObj = {
        'op': 'queue',
        'score': score,
        'scope': scope,
        'players': None,
        'params': {}
    }

    reqObj['players'] = list(target)

    if args is not None:
        reqObj['params'] = args

    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(sockname)
    f = sock.makefile()
    try:
        f.write(json.dumps(reqObj))
        f.flush()
        resp = json.load(f)

        if resp[0] == 'OK':
            return resp[1]
        else:
            raise ServerError(resp[1])
    finally:
        sock.close()

def get_status(jobid, sockname=DEFAULT_SOCKET_PATH):
    reqObj = {
        'op': 'status',
        'id': jobid
    }
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(sockname)
    f = sock.makefile()
    try:
        f.write(json.dumps(reqObj))
        f.flush()
        resp = json.load(f)

        if resp[0] == 'OK':
            return resp[1]
        else:
            raise ServerError(resp[1])
    finally:
        sock.close()

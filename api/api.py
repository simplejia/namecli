# -*- coding: utf-8 -*-
#! /usr/bin/env python

import time
import socket

class NameApi:
    def __init__(self, ip, port):
        self.ip = ip
        self.port = port
    
    def proc(self, name):
        retried = False
        while True:
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            try:
                seq = int(round(time.time() * 1e6))
                sock.sendto(b'%d,%s' % (seq, name), (self.ip, self.port))
                sock.settimeout(0.02)
                buf=sock.recv(64)
                _seq, _, ipport = buf.partition(",")
                if seq != int(_seq):
                    if retried is False:
                        retried = True
                        continue
                    return -1, ""
                if ipport == "":
                    if retried is False:
                        retried = True
                        continue
                    return -2, ""
                return 0, ipport
            except:
                return -3, ""
            finally:
                sock.close()

if __name__ == '__main__':
    ip = '127.0.0.1'
    port = 8328

    obj = NameApi(ip, port)

    name = "namesrv.ns"
    ret, ipport = obj.proc(name)
    print(ret, ipport)
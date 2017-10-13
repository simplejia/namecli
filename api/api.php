<?php

class NameApi
{
    private $ip;
    private $port;

    public function __construct($ip, $port) {
        $this->ip = $ip;
        $this->port = $port;
    }

    public function proc($name, &$ipport) {
        $retried = false;
    again:
        $ret = 0;
        $sock = socket_create(AF_INET, SOCK_DGRAM, 0);
        if (!$sock) {
            return -1;
        }

        $seq = (string)microtime(true);
        $msg = sprintf("%s,%s", $seq, $name);
        if (!socket_sendto($sock, $msg, strlen($msg), 0, $this->ip, $this->port)) {
            socket_close($sock);
            return -2;
        }

        socket_set_option($sock, SOL_SOCKET, SO_RCVTIMEO, array("sec"=>0, "usec"=>20000));
        if (!socket_recv($sock, $buf, 64, 0)) {
            socket_close($sock);
            return -3;
        }

        list($_seq, $ipport) = explode(",", $buf);
        if ($seq != $_seq) {
            socket_close($sock);
            if (!$retried) {
                $retried = true;
                goto again;
            }
            return -4;
        }

        socket_close($sock);
        if ($ipport == "") {
            if (!$retried) {
                $retried = true;
                goto again;
            }
            return -5;
        }
        return 0;
    }
}

$ip = "127.0.0.1";
$port = 8328;
$obj = new NameApi($ip, $port);

$name = "namesrv.ns";
$ipport = ""; 

$ret = $obj->proc($name, $ipport);
if ($ret != 0) {
    printf("proc() error: %d\n", $ret);
    exit(-1);
}

var_dump($ipport);

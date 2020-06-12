package api

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"
)

var (
	// agent addr
	Addr = "127.0.0.1:8328"
	seq  uint32
)

func Name(name string) (addr string, err error) {
	retried := false
again:
	conn, err := net.Dial("udp", Addr)
	if err != nil {
		return
	}
	defer conn.Close()

	seq := atomic.AddUint32(&seq, 1)
	req := fmt.Sprintf("%d,%s", seq, name)
	conn.SetWriteDeadline(time.Now().Add(time.Second * 10))
	_, err = conn.Write([]byte(req))
	if err != nil {
		return
	}

	rsp := [64]byte{}
	conn.SetReadDeadline(time.Now().Add(time.Second * 10))
	n, err := conn.Read(rsp[:])
	if err != nil || n <= 0 {
		return
	}

	_rsp := rsp[:n]
	i := bytes.IndexByte(_rsp, ',')
	if i == -1 {
		// no expected
		return
	}
	_seq, _ := strconv.ParseUint(string(_rsp[:i]), 10, 32)
	if seq != uint32(_seq) {
		if !retried {
			retried = true
			goto again
		}
		err = errors.New("seq invalid from namesrv")
		return
	}

	addr = string(_rsp[i+1:])
	if addr == "" {
		if !retried {
			retried = true
			goto again
		}
		err = fmt.Errorf("no addr found from namesrv: %s", name)
		return
	}

	return
}

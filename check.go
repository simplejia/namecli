package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/simplejia/lc"
	"github.com/simplejia/utils"
)

var (
	MutexRemote sync.Mutex
	MapRemote   = map[string]bool{}
	MutexLocal  sync.Mutex
	MapLocal    = map[string]bool{}
)

func init() {
	LocalIp := utils.LocalIp
	if LocalIp == "" {
		log.Fatalln("get localip error")
	}
	log.Println("localip:", LocalIp)

	go CheckLocalConn(LocalIp)
}

func GetRelsFromIp(ip, addr string, rdOld *RespData) (rdNew *RespData) {
	if ip == "" || addr == "" {
		return
	}

	cc := ""
	if rdOld != nil {
		cc = rdOld.CheckCode
	}
	url := fmt.Sprintf("http://%s/%s?ip=%s&cc=%s", addr, "relation/getsFromIp", ip, cc)

	statusCode := new(int)
	body, err := utils.Get(&utils.GPP{Uri: url, Timeout: 3 * time.Second, StatusCodeRet: statusCode})
	if err != nil {
		log.Printf("http get error: %v, url: %s\n", err, url)
		return
	}
	if *statusCode == http.StatusNotModified {
		rdNew = rdOld
		return
	}

	if *statusCode != http.StatusOK {
		log.Printf("http code not 200: %d, resp: %s\n", *statusCode, body)
		return
	}

	if err := json.Unmarshal(body, &rdNew); err != nil {
		log.Printf("http resp invalid: %v, url: %s\n", err, url)
		return
	}
	return
}

func ReportOff(ipport string, off bool, addr string) {
	if ipport == "" || addr == "" {
		return
	}

	url := fmt.Sprintf("http://%s/%s?ipport=%s&off=%t", addr, "relation/reportOff", ipport, off)
	if _, err := utils.Get(&utils.GPP{Uri: url, Timeout: 3 * time.Second}); err != nil {
		log.Printf("http get error: %v, url: %s\n", err, url)
		return
	}

	return
}

func CheckLocalConn(ip string) {
	var rd *RespData

	for {
		time.Sleep(time.Second * 5)

		rd = GetRelsFromIp(ip, GetSrvAddr(), rd)
		if rd == nil {
			continue
		}

		MutexLocal.Lock()
		for _, rel := range rd.Rels {
			addr := rel.JoinHostPort()
			if MapLocal[addr] {
				continue
			}
			MapLocal[addr] = true
			if rel.Udp {
				go CheckConnUdp(addr, false)
			} else {
				go CheckConnTcp(addr, false)
			}
		}
		MutexLocal.Unlock()
	}
}

func CheckConnTcp(addr string, isRemote bool) {
	defer func() {
		if isRemote {
			MutexRemote.Lock()
			delete(MapRemote, addr)
			MutexRemote.Unlock()
		} else {
			MutexLocal.Lock()
			delete(MapLocal, addr)
			MutexLocal.Unlock()
		}
	}()

	for d, bt := [1]byte{}, time.Now(); time.Since(bt) < time.Minute; {
		c, err := net.DialTimeout("tcp", addr, time.Second*10)
		if err != nil {
			if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
				continue
			}
			if isRemote {
				lc.Set(GetOffKey(addr), true, NameExpire)
			} else {
				ReportOff(addr, true, GetSrvAddr())
			}
			break
		}

		c.(*net.TCPConn).SetLinger(0)

		if isRemote {
			lc.Set(GetOffKey(addr), false, NameExpire)
		} else {
			ReportOff(addr, false, GetSrvAddr())
		}

		readt := time.Now()
		c.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := c.Read(d[:]); err == io.EOF {
			if c, err := net.DialTimeout("tcp", addr, time.Second*10); err != nil {
				if opErr, ok := err.(*net.OpError); ok {
					if sysErr, ok := opErr.Err.(*os.SyscallError); ok && sysErr.Err == syscall.ECONNREFUSED {
						if isRemote {
							lc.Set(GetOffKey(addr), true, NameExpire)
						} else {
							ReportOff(addr, true, GetSrvAddr())
						}
					}
				}
			} else {
				c.(*net.TCPConn).SetLinger(0)
				c.Close()
			}
		}

		c.Close()

		if time.Since(readt) < time.Second {
			time.Sleep(time.Second)
		}
	}

	return
}

func CheckConnUdp(addr string, isRemote bool) {
	defer func() {
		if isRemote {
			MutexRemote.Lock()
			delete(MapRemote, addr)
			MutexRemote.Unlock()
		} else {
			MutexLocal.Lock()
			delete(MapLocal, addr)
			MutexLocal.Unlock()
		}
	}()

	c, err := net.Dial("udp", addr)
	if err != nil {
		goto fail
	}
	defer c.Close()

	for d, bt := [1]byte{}, time.Now(); time.Since(bt) < time.Minute; {
		if _, err := c.Write(nil); err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if sysErr, ok := opErr.Err.(*os.SyscallError); ok && sysErr.Err == syscall.ECONNREFUSED {
					goto fail
				}
			}
		}

		readt := time.Now()
		c.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := c.Read(d[:]); err != nil {
			if opErr, ok := err.(*net.OpError); ok {
				if sysErr, ok := opErr.Err.(*os.SyscallError); ok && sysErr.Err == syscall.ECONNREFUSED {
					goto fail
				}
			}
		}

		if isRemote {
			lc.Set(GetOffKey(addr), false, NameExpire)
		} else {
			ReportOff(addr, false, GetSrvAddr())
		}

		if time.Since(readt) < time.Second {
			time.Sleep(time.Second)
		}
	}

	return

fail:
	if isRemote {
		lc.Set(GetOffKey(addr), true, NameExpire)
	} else {
		ReportOff(addr, true, GetSrvAddr())
	}
}

func CheckRemoteConn(rels []*Relation) {
	MutexRemote.Lock()
	for _, rel := range rels {
		addr := rel.JoinHostPort()
		if MapRemote[addr] {
			continue
		}
		MapRemote[addr] = true
		if rel.Udp {
			go CheckConnUdp(addr, true)
		} else {
			go CheckConnTcp(addr, true)
		}
	}
	MutexRemote.Unlock()
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	resp, err := HttpClient.Get(url)
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			log.Printf("http get error: %v, url: %s\n", err, url)
		}
	}()
	if err != nil {
		return
	}
	code := resp.StatusCode
	if code == http.StatusNotModified {
		rdNew = rdOld
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if code != http.StatusOK {
		err = fmt.Errorf("http code not 200: %d, resp: %s", code, body)
		return
	}

	err = json.Unmarshal(body, &rdNew)
	return
}

func ReportOff(ipport string, off bool, addr string) {
	if ipport == "" || addr == "" {
		return
	}

	url := fmt.Sprintf("http://%s/%s?ipport=%s&off=%t", addr, "relation/reportOff", ipport, off)
	resp, err := HttpClient.Get(url)
	defer func() {
		if resp != nil {
			resp.Body.Close()
		}
		if err != nil {
			log.Printf("http get error: %v, url: %s\n", err, url)
		}
	}()
	if err != nil {
		return
	}
	code := resp.StatusCode
	if code != http.StatusOK {
		err = fmt.Errorf("http code not 200: %d", code)
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

	retried := false
again:
	c, err := net.DialTimeout("tcp", addr, time.Second*10)
	if err != nil {
		if opErr, ok := err.(*net.OpError); ok {
			if sysErr, ok := opErr.Err.(*os.SyscallError); ok && sysErr.Err == syscall.ECONNREFUSED {
				if isRemote {
					lc.Set(GetOffKey(addr), true, NameExpire)
				} else {
					ReportOff(addr, true, GetSrvAddr())
				}
			}
		}
		return
	}
	defer c.Close()

	if isRemote {
		lc.Set(GetOffKey(addr), false, NameExpire)
	} else {
		ReportOff(addr, false, GetSrvAddr())
	}

	for d, bt := [64]byte{}, time.Now(); time.Since(bt) < time.Minute; {
		c.SetReadDeadline(time.Now().Add(time.Second * 5))
		if _, err := c.Read(d[:]); err == io.EOF {
			if !retried {
				retried = true
				goto again
			}
			break
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
		return
	}
	defer c.Close()

	for d, bt := [64]byte{}, time.Now(); time.Since(bt) < time.Minute; {
		var err error
		c.Write(nil)
		for {
			c.SetReadDeadline(time.Now().Add(time.Second * 5))
			if _, err = c.Read(d[:]); err != nil {
				break
			}
		}
		if opErr, ok := err.(*net.OpError); ok {
			if opErr.Timeout() {
				if isRemote {
					lc.Set(GetOffKey(addr), false, NameExpire)
				} else {
					ReportOff(addr, false, GetSrvAddr())
				}
			} else if sysErr, ok := opErr.Err.(*os.SyscallError); ok && sysErr.Err == syscall.ECONNREFUSED {
				if isRemote {
					lc.Set(GetOffKey(addr), true, NameExpire)
				} else {
					ReportOff(addr, true, GetSrvAddr())
				}
				break
			}
		}
	}
	return
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

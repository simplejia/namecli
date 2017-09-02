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
	"time"

	"github.com/simplejia/lc"
	"github.com/simplejia/utils"
)

var (
	Mu sync.Mutex
	M  = map[string]bool{}
)

func init() {
	LocalIp := utils.LocalIp
	if LocalIp == "" {
		log.Println("get localip error")
		os.Exit(-1)
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
		time.Sleep(time.Second * 10)

		rd = GetRelsFromIp(ip, GetSrvAddr(), rd)
		if rd == nil {
			continue
		}
		for _, rel := range rd.Rels {
			switch rel.Udp {
			case true:
				addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", rel.Port))
				ln, err := net.ListenUDP("udp", addr)
				if err == nil {
					ReportOff(rel.JoinHostPort(), true, GetSrvAddr())
					ln.Close()
				} else {
					ReportOff(rel.JoinHostPort(), false, GetSrvAddr())
				}
			case false:
				addr, _ := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", rel.Port))
				ln, err := net.ListenTCP("tcp", addr)
				if err == nil {
					ReportOff(rel.JoinHostPort(), true, GetSrvAddr())
					ln.Close()
				} else {
					ReportOff(rel.JoinHostPort(), false, GetSrvAddr())
				}
			}
		}
	}
}

func CheckRemoteConn(addr string) {
	defer func() {
		Mu.Lock()
		delete(M, addr)
		Mu.Unlock()
	}()

	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			return
		}
		lc.Set(GetOffKey(addr), true, NameExpire)
		return
	}
	if v, _ := lc.Get(GetOffKey(addr)); v != nil {
		lc.Set(GetOffKey(addr), false, NameExpire)
	}

	for d, bt := [1]byte{}, time.Now(); time.Since(bt) < time.Minute*5; {
		c.SetReadDeadline(time.Now().Add(time.Second * 10))
		_, err = c.Read(d[:])
		if err == nil {
			break
		}
		if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
			continue
		}
		if err == io.EOF {
			lc.Set(GetOffKey(addr), true, NameExpire)
			break
		}
	}
	c.Close()
	return
}

func Check(name string, rels []*Relation) {
	Mu.Lock()
	defer Mu.Unlock()

	for _, rel := range rels {
		if rel.Udp {
			continue
		}
		addr := rel.JoinHostPort()
		if M[addr] {
			continue
		}
		M[addr] = true
		go CheckRemoteConn(addr)
	}
}

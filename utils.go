package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/simplejia/lc"
	"github.com/simplejia/utils"
)

var (
	BodySeq    = byte(',')
	NameExpire = time.Second * 5
	OnPrefix   = "on:"
	OffPrefix  = "off:"
)

func GetOnKey(name string) string {
	return OnPrefix + name
}

func GetOffKey(name string) string {
	return OffPrefix + name
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

type Relation struct {
	Name   string
	Ip     string
	Port   int
	Udp    bool
	Weight int
}

func (rel *Relation) JoinHostPort() string {
	if rel == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", rel.Ip, rel.Port)
}

type RespData struct {
	Rels          []*Relation
	CheckCode     string `json:"cc"`
	Num           int    `json:"-"`
	Gcd           int    `json:"-"`
	MaxWeight     int    `json:"-"`
	CurrentIndex  int    `json:"-"`
	CurrentWeight int    `json:"-"`
}

func (rd *RespData) GetGcd() int {
	if rd == nil {
		return 0
	}
	divisor := -1
	for _, rel := range rd.Rels {
		if divisor == -1 {
			divisor = rel.Weight
		} else {
			divisor = gcd(divisor, rel.Weight)
		}
	}

	return divisor
}

func (rd *RespData) GetMaxWeight() int {
	if rd == nil {
		return 0
	}
	max := -1
	for _, rel := range rd.Rels {
		if rel.Weight > max {
			max = rel.Weight
		}
	}

	return max
}

func (rd *RespData) Copy() (rdNew *RespData) {
	if rd == nil {
		return
	}
	rdNew = &RespData{
		Rels:          rd.Rels,
		CheckCode:     rd.CheckCode,
		CurrentIndex:  rd.CurrentIndex,
		CurrentWeight: rd.CurrentWeight,
		Gcd:           rd.GetGcd(),
		MaxWeight:     rd.GetMaxWeight(),
	}
	return
}

func (rd *RespData) NextIndex() int {
	index := rd.CurrentIndex
	weight := rd.CurrentWeight
	for {
		index = (index + 1) % len(rd.Rels)
		if index == 0 {
			weight -= rd.Gcd
			if weight <= 0 {
				weight = rd.MaxWeight
			}
		}
		rel := rd.Rels[index]
		if rel.Weight >= weight {
			break
		}
	}

	rd.CurrentIndex = index
	rd.CurrentWeight = weight
	return index
}

func (rd *RespData) GetAddr() (addr string) {
	if rd == nil {
		return
	}
	rels := rd.Rels
	if len(rels) == 0 {
		return
	}

	for len(rels) > 0 {
		i := rd.NextIndex() % len(rels)
		rel := rels[i]
		_addr := rel.JoinHostPort()
		vLc, _ := lc.Get(GetOffKey(_addr))
		if vLc == nil || !vLc.(bool) {
			addr = _addr
			break
		}
		_rels := append([]*Relation{}, rels[:i]...)
		rels = append(_rels, rels[i+1:]...)
	}

	return
}

func SplitBody(body []byte) (seq, name []byte) {
	pos := bytes.IndexByte(body, BodySeq)
	if pos == -1 {
		log.Println("request body must be splited by ','")
		return
	}
	seq, name = body[:pos], body[pos+1:]
	return
}

func JoinBody(seq, name []byte) (body []byte) {
	body = make([]byte, len(seq)+len(name)+1)
	n := copy(body, seq)
	body[n] = BodySeq
	n++
	copy(body[n:], name)
	return
}

func GetSrvAddr() (addr string) {
	var rd *RespData
	rdLc, ok := lc.Get(GetOnKey(SrvName))
	if rdLc != nil {
		rd = rdLc.(*RespData)
		rd.Num++
		addr = rd.GetAddr()
	}
	if ok && addr != "" {
		return
	}

	if addr == "" {
		addr = SrvAddr
		rd = GetRelsFromName(SrvName, addr, rd)
		if rd != nil {
			addr = rd.GetAddr()
			go func() {
				lc.Set(GetOnKey(SrvName), rd, NameExpire)
				CheckRemoteConn(rd.Rels)
			}()
		}
	} else {
		go func() {
			rd = GetRelsFromName(SrvName, addr, rd)
			if rd != nil {
				lc.Set(GetOnKey(SrvName), rd, NameExpire)
				CheckRemoteConn(rd.Rels)
			}
		}()
	}

	return
}

func GetRelsFromName(name, addr string, rdOld *RespData) (rdNew *RespData) {
	if name == "" || addr == "" {
		return
	}

	cc, num := "", 0
	if rdOld != nil {
		cc, num = rdOld.CheckCode, rdOld.Num
	}
	url := fmt.Sprintf("http://%s/%s?name=%s&cc=%s&num=%d", addr, "relation/getsFromName", name, cc, num)

	statusCode := new(int)
	body, err := utils.Get(&utils.GPP{Uri: url, Timeout: 3 * time.Second, StatusCodeRet: statusCode})
	if err != nil {
		log.Printf("http get error: %v, url: %s\n", err, url)
		return
	}
	if *statusCode == http.StatusNotModified {
		rdNew = rdOld.Copy()
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
	rdNew.CurrentIndex = -1
	rdNew.Gcd = rdNew.GetGcd()
	rdNew.MaxWeight = rdNew.GetMaxWeight()
	return
}

func GetAddrFromName(name string) (addr string) {
	var rd *RespData
	rdLc, ok := lc.Get(GetOnKey(name))
	if rdLc != nil {
		rd = rdLc.(*RespData)
		addr = rd.GetAddr()
		rd.Num++
	}
	if ok && addr != "" {
		return
	}

	if addr == "" {
		rd = GetRelsFromName(name, GetSrvAddr(), rd)
		if rd != nil {
			addr = rd.GetAddr()
			rd.Num++
			go func() {
				lc.Set(GetOnKey(name), rd, NameExpire)
				CheckRemoteConn(rd.Rels)
			}()
		}
	} else {
		go func() {
			rd = GetRelsFromName(name, GetSrvAddr(), rd)
			if rd != nil {
				lc.Set(GetOnKey(name), rd, NameExpire)
				CheckRemoteConn(rd.Rels)
			}
		}()
	}

	return
}

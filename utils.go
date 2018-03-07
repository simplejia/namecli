package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/simplejia/lc"
)

var (
	BodySeq    = byte(',')
	NameExpire = time.Second * 5
	HttpClient = &http.Client{Timeout: time.Second}
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
			weight = weight - rd.Gcd
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

func ShuffleAddrs(addrs []string) {
	for i := len(addrs) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		addrs[i], addrs[j] = addrs[j], addrs[i]
	}
}

func GetSrvAddr() (addr string) {
	var rd *RespData
	rdLc, ok := lc.Get(GetOnKey(SrvName))
	if rdLc != nil {
		rd = rdLc.(*RespData)
		rd.Num++
		addr = rd.GetAddr()
	}

	if addr == "" {
		addr = SrvAddr
	}

	if ok {
		return
	}

	go func() {
		addrs := []string{}
		if rd != nil {
			for _, rel := range rd.Rels {
				addrs = append(addrs, rel.JoinHostPort())
			}
			ShuffleAddrs(addrs)
		}
		addrs = append(addrs, SrvAddr)
		for _, addr := range addrs {
			rd = GetRelsFromName(SrvName, addr, rd)
			if rd != nil {
				lc.Set(GetOnKey(SrvName), rd, NameExpire)
				Check(SrvName, rd.Rels)
				break
			}
		}
	}()

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
		rdNew = rdOld.Copy()
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

func GetAddrFromName(name string) (addr string) {
	var rd *RespData
	rdLc, ok := lc.Get(GetOnKey(name))
	if rdLc != nil {
		rd = rdLc.(*RespData)
		rd.Num++
		addr = rd.GetAddr()
	}
	if ok && addr != "" {
		return
	}

	if addr == "" {
		rd = GetRelsFromName(name, GetSrvAddr(), rd)
		if rd != nil {
			lc.Set(GetOnKey(name), rd, NameExpire)
			Check(name, rd.Rels)
			addr = rd.GetAddr()
		}
	} else {
		go func() {
			rd = GetRelsFromName(name, GetSrvAddr(), rd)
			if rd != nil {
				lc.Set(GetOnKey(name), rd, NameExpire)
				Check(name, rd.Rels)
			}
		}()
	}

	return
}

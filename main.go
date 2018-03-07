// 名字服务(client).
// author: simplejia
// date: 2017/07/30
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"

	"github.com/simplejia/lc"
)

var (
	NumProcs int
	Port     int
	SrvAddr  string
	SrvName  string
)

func init() {
	lc.Init(1e5)
}

func main() {
	log.Println("main()")

	flag.IntVar(&NumProcs, "num_procs", 0, "specify the concurrent process num")
	flag.IntVar(&Port, "port", 8328, "specify the listening port")
	flag.StringVar(&SrvAddr, "srv_addr", "", "specify the namesrv addr")
	flag.StringVar(&SrvName, "srv_name", "", "specify the namesrv name")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "A nameserver\n")
		fmt.Fprintf(os.Stderr, "version: 1.0, Created by simplejia [7/2017]\n\n")
		fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	if Port == 0 || SrvAddr == "" || SrvName == "" {
		flag.Usage()
		os.Exit(-1)
	}

	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", Port))
	if err != nil {
		log.Fatalln("net.ResolveUDPAddr error:", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalln("net.ListenUDP error:", err)
	}
	defer conn.Close()

	if NumProcs <= 0 {
		NumProcs = runtime.NumCPU()
	}

	for i := 0; i < NumProcs; i++ {
		go func() {
			request := make([]byte, 1024)
			for {
				readLen, raddr, err := conn.ReadFrom(request)
				if err != nil || readLen <= 0 {
					continue
				}

				body := request[:readLen]
				seq, name := SplitBody(body)
				addr := GetAddrFromName(string(name))
				body = JoinBody(seq, []byte(addr))
				conn.WriteTo(body, raddr)
			}
		}()
	}

	select {}
}

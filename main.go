package main

import (
	"fmt"
	"gingle-rpc/client"
	"gingle-rpc/server"
	"log"
	"net"
	"sync"
	"time"
)

var DefaultServer *server.Server = server.NewServer()

func StartServer(addr chan string) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("start server error: %v\n", err)
	}
	log.Printf("start server succeed: %s\n", lis.Addr().String())
	addr <- lis.Addr().String()

	DefaultServer.Accept(lis)
}

func ClientCall(i int, conn *client.Client) {
	args := fmt.Sprintf("%d", i)
	reply := ""
	if err := conn.Call("Foo.Sum", args, &reply); err != nil {
		log.Fatalf("client call error: %v\n", err)
	}
	log.Printf("req: %s, res: %s \n", args, reply)
}

func main() {
	log.SetFlags(0)
	addr := make(chan string)

	go StartServer(addr)
	conn, _ := client.Dial("tcp", <-addr)
	defer func() { _ = conn.Close() }()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ClientCall(i, conn)
		}(i)
	}
}

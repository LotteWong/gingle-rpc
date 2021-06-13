package main

import (
	"context"
	"gingle-rpc/client"
	"gingle-rpc/loadbalance"
	"gingle-rpc/server"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// Service Method Part

type Foo int

type Args struct {
	Num1 int
	Num2 int
}

func (f *Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

// Rpc Server Part

var DefaultServer *server.Server = server.NewServer()

func StartServer(addr chan string) {
	var foo Foo
	if err := DefaultServer.RegisterService(&foo); err != nil {
		log.Fatalf("register service error: %v\n", err)
	}
	log.Printf("register service succeed: %v\n", foo)

	lis, err := net.Listen("tcp", ":9999")
	if err != nil {
		log.Fatalf("start server error: %v\n", err)
	}
	log.Printf("start server succeed: %s\n", lis.Addr().String())

	DefaultServer.HandleHTTP()
	addr <- lis.Addr().String()
	http.Serve(lis, nil)
}

// Rpc Client Part

func StartClient(addr chan string) {
	conn, _ := client.DialHTTP("tcp", <-addr)
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
	wg.Wait()
}

func ClientCall(i int, conn *client.Client) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	serviceMethod := "Foo.Sum"
	args := &Args{Num1: i, Num2: i * i}
	reply := 0
	if err := conn.Call(ctx, serviceMethod, args, &reply); err != nil {
		log.Fatalf("client call error: %v\n", err)
	}
	log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
}

func StartXClientByPeerToPeer(servers []string) {
	lb := loadbalance.NewLoadBalanceWithClientDiscovery(servers)
	xc := client.NewXClient(lb, loadbalance.Random, nil)
	defer func() { _ = xc.Close() }()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			XClientCall("p2p", xc, i)
		}(i)
	}
	wg.Wait()
}

func StartXClientByBroadcast(servers []string) {
	lb := loadbalance.NewLoadBalanceWithClientDiscovery(servers)
	xc := client.NewXClient(lb, loadbalance.Random, nil)
	defer func() { _ = xc.Close() }()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			XClientCall("broadcast", xc, i)
		}(i)
	}
	wg.Wait()
}

func XClientCall(xType string, xc *client.XClient, i int) {
	ctx := context.Background()
	serviceMethod := "Foo.Sum"
	args := &Args{Num1: i, Num2: i * i}
	reply := 0

	var err error
	switch xType {
	case "p2p":
		err = xc.PeerToPeer(ctx, serviceMethod, args, &reply)
	case "broadcast":
		err = xc.Broadcast(ctx, serviceMethod, args, &reply)
	}

	if err != nil {
		log.Printf("%s %s error: %v", xType, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", xType, serviceMethod, args.Num1, args.Num2, reply)
	}
}

// Main Demo Part

func main() {
	log.SetFlags(0)

	addr := make(chan string)
	go StartClient(addr)
	StartServer(addr)

	ch1 := make(chan string)
	ch2 := make(chan string)
	go StartServer(ch1)
	go StartServer(ch2)
	addr1 := <-ch1
	addr2 := <-ch2
	StartXClientByPeerToPeer([]string{addr1, addr2})
	StartXClientByBroadcast([]string{addr1, addr2})
}

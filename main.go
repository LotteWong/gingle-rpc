package main

import (
	"context"
	"gingle-rpc/client"
	"gingle-rpc/server"
	"log"
	"net"
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

	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("start server error: %v\n", err)
	}
	log.Printf("start server succeed: %s\n", lis.Addr().String())
	addr <- lis.Addr().String()

	DefaultServer.Accept(lis)
}

// Rpc Client Part

func ClientCall(i int, conn *client.Client) {
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	args := &Args{Num1: i, Num2: i * i}
	reply := 0
	if err := conn.Call(ctx, "Foo.Sum", args, &reply); err != nil {
		log.Fatalf("client call error: %v\n", err)
	}
	log.Printf("%d + %d = %d", args.Num1, args.Num2, reply)
}

// Main Demo Part

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

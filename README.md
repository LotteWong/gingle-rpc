# gingle-rpc

A simple net/rpc-like rpc framework implemented by Golang.

---

## Features

- [x] Support Multiple Encode and Decode Protocols

- [x] Support Multiple Network Protocols

- [x] Reflected Server and Concurrent Client

- [x] Timeout Handle Mechanism

- [x] Load Balance with Client or Server Discovery

- [x] Registry Center with Health Check

## Quick Start

### Main Demo Sample

```go
func main() {
  log.SetFlags(0)
  registryAddr := "http://localhost:9999/gingle/registry"

  addr := make(chan string)
  go StartClient(addr)
  StartServer(addr, registryAddr)

  ch1 := make(chan string)
  ch2 := make(chan string)
  go StartServer(ch1, registryAddr)
  go StartServer(ch2, registryAddr)
  StartXClientByPeerToPeer(registryAddr)
  StartXClientByBroadcast(registryAddr)
}
```

## TODOs

- [ ] Unit Tests

- [ ] More Load Balance Algorithms

- [ ] Refactor Codes

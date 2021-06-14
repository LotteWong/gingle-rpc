package server

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// RegistryServerItem includes address, start at and end at
type RegistryServerItem struct {
	Addr    string
	StartAt time.Time
	EndAt   time.Time
}

// RegistryServer includes servers, timeout and mutex
type RegistryServer struct {
	servers map[string]*RegistryServerItem

	timeout time.Duration
	mu      sync.Mutex
}

// NewRegistryServer is to create registry server
func NewRegistryServer(timeout time.Duration) *RegistryServer {
	return &RegistryServer{
		servers: make(map[string]*RegistryServerItem),
		timeout: timeout,
	}
}

func (s *RegistryServer) registerServer(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	server, ok := s.servers[addr]
	if ok {
		server.StartAt = time.Now()
	} else {
		s.servers[addr] = &RegistryServerItem{
			Addr:    addr,
			StartAt: time.Now(),
		}
	}
}

func (s *RegistryServer) exploreServers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var addrs []string
	for addr, item := range s.servers {
		if s.timeout == 0 || item.StartAt.Add(s.timeout).After(time.Now()) { // not limit or not timeout, server is alive
			addrs = append(addrs, addr)
		} else { // with limit and already timeout, server is not alive
			delete(s.servers, addr)
		}
	}

	sort.Strings(addrs)
	return addrs
}

// ServeHTTP is to explore servers or register server
func (s *RegistryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// explore servers
		servers := strings.Join(s.exploreServers(), ",")
		w.Header().Set("X-Gingle-Rpc-Servers", servers)
	case "POST":
		// register server
		addr := r.Header.Get("X-Gingle-Rpc-Server")
		if addr == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.registerServer(addr)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// HealthCheckOnce is to do registry center health check once
func HealthCheckOnce(serverAddr, clientAddr string) error {
	httpClient := &http.Client{}
	req, _ := http.NewRequest("POST", serverAddr, nil)
	req.Header.Set("X-Gingle-Rpc-Server", clientAddr)

	if _, err := httpClient.Do(req); err != nil {
		return err
	}
	return nil
}

// HealthCheckPeriodically is to do registry center health check periodically
func HealthCheckPeriodically(serverAddr, clientAddr string, period time.Duration) {
	if period == 0 {
		period = defaultPeriod
	}

	var err error
	err = HealthCheckOnce(serverAddr, clientAddr)
	go func() {
		t := time.NewTicker(period)
		for err == nil {
			<-t.C
			err = HealthCheckOnce(serverAddr, clientAddr)
		}
	}()
}

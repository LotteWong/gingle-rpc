package loadbalance

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// LoadBalanceWithClientDiscovery includes index, servers, rand func and mutex
type LoadBalanceWithClientDiscovery struct {
	idx     int
	servers []string

	r  *rand.Rand
	mu sync.RWMutex
}

var _ LoadBalance = (*LoadBalanceWithClientDiscovery)(nil)

// NewLoadBalanceWithClientDiscovery is create load balance with client discovery
func NewLoadBalanceWithClientDiscovery(servers []string) *LoadBalanceWithClientDiscovery {
	return &LoadBalanceWithClientDiscovery{
		servers: servers,
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	// lb.idx = lb.r.Intn(math.MaxInt32-1)
}

// Refresh is to refresh servers from remote
func (lb *LoadBalanceWithClientDiscovery) Refresh() error {
	return nil
}

// Update is to update servers from local
func (lb *LoadBalanceWithClientDiscovery) Update(servers []string) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.servers = servers
	return nil
}

// GetOne is to get one server using the load balance algorithm
func (lb *LoadBalanceWithClientDiscovery) GetOne(mode LbAlgo) (string, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	n := len(lb.servers)
	if n == 0 {
		return "", fmt.Errorf("discovery: no available servers")
	}

	switch mode {
	case Random:
		return lb.selectRandom()
	case RoundRobin:
		return lb.selectRoundRobin()
	default:
		return "", fmt.Errorf("discovery: no such load balance algorithm mode")
	}
}

func (lb *LoadBalanceWithClientDiscovery) selectRandom() (string, error) {
	n := len(lb.servers)
	server := lb.servers[lb.r.Intn(n)]
	return server, nil
}

func (lb *LoadBalanceWithClientDiscovery) selectRoundRobin() (string, error) {
	n := len(lb.servers)
	server := lb.servers[lb.idx%n]
	lb.idx = (lb.idx + 1) % n
	return server, nil
}

// GetAll is to get all servers
func (lb *LoadBalanceWithClientDiscovery) GetAll() ([]string, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	servers := make([]string, len(lb.servers))
	copy(servers, lb.servers)
	return servers, nil
}

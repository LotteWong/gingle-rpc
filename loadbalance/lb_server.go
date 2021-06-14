package loadbalance

import (
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second

// LoadBalanceWithServerDiscovery includes registry address, load balance with client discovery, timeout and update at
type LoadBalanceWithServerDiscovery struct {
	registryAddr string
	*LoadBalanceWithClientDiscovery

	timeout  time.Duration
	updateAt time.Time
}

var _ LoadBalance = (*LoadBalanceWithServerDiscovery)(nil)

// NewLoadBalanceWithServerDiscovery is create load balance with server discovery
func NewLoadBalanceWithServerDiscovery(registryAddr string, timeout time.Duration) *LoadBalanceWithServerDiscovery {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &LoadBalanceWithServerDiscovery{
		registryAddr:                   registryAddr,
		LoadBalanceWithClientDiscovery: NewLoadBalanceWithClientDiscovery(make([]string, 0)),
		timeout:                        timeout,
	}
}

// Refresh is to refresh servers from remote
func (lb *LoadBalanceWithServerDiscovery) Refresh() error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if lb.updateAt.Add(lb.timeout).After(time.Now()) {
		return nil
	}

	res, err := http.Get(lb.registryAddr)
	if err != nil {
		return err
	}

	servers := strings.Split(res.Header.Get("X-Gingle-Rpc-Servers"), ",")
	lb.servers = make([]string, 0, len(servers))
	for _, server := range servers {
		if strings.TrimSpace(server) != "" {
			lb.servers = append(lb.servers, strings.TrimSpace(server))
		}
	}
	lb.updateAt = time.Now()

	return nil
}

// Update is to update servers from local
func (lb *LoadBalanceWithServerDiscovery) Update(servers []string) error {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.servers = servers
	lb.updateAt = time.Now()
	return nil
}

// GetOne is to get one server using the load balance algorithm
func (lb *LoadBalanceWithServerDiscovery) GetOne(mode LbAlgo) (string, error) {
	if err := lb.Refresh(); err != nil {
		return "", err
	}
	return lb.LoadBalanceWithClientDiscovery.GetOne(mode)
}

// GetAll is to get all servers
func (lb *LoadBalanceWithServerDiscovery) GetAll() ([]string, error) {
	if err := lb.Refresh(); err != nil {
		return nil, err
	}
	return lb.LoadBalanceWithClientDiscovery.GetAll()
}

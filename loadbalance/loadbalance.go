package loadbalance

type LbAlgo int

const (
	Random LbAlgo = iota
	RoundRobin
)

// LoadBalance support to refresh, update, get one server or get all servers
type LoadBalance interface {
	Refresh() error        // from remote
	Update([]string) error // from local
	GetOne(LbAlgo) (string, error)
	GetAll() ([]string, error)
}

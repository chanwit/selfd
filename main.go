package main

import (
	"fmt"
	"os"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/serf"
)

type Agent struct {
	conf *serf.Config
	eventCh chan serf.Event
	serf *serf.Serf

	shutdown     bool
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex

	isManager bool
}

func CreateAsManager() (*Agent, error) {
	agent, err := Create()
	agent.isManager = true
	return agent, err
}

func Create() (*Agent, error) {
	conf := serf.DefaultConfig()
	conf.Init()
	conf.NodeName = os.Getenv("HOST")
	conf.Tags["DOCKER_HOST"] = Os.Getenv("DOCKER_HOST")

	logOutput := log.StandardLogger().Out

	// Setup the underlying loggers
	conf.MemberlistConfig.LogOutput = logOutput
	conf.LogOutput = logOutput

	// Create a channel to listen for events from Serf
	eventCh := make(chan serf.Event, 64)
	conf.EventCh = eventCh

	// support only LAN configuration at the moment
	conf.MemberlistConfig = memberlist.DefaultLANConfig()
	conf.MemberlistConfig.BindAddr = "0.0.0.0"
	conf.MemberlistConfig.BindPort = 3388

	// Setup the agent
	agent := &Agent{
		conf: conf,
		eventCh: eventCh,
		isManager: false,
		shutdownCh: make(chan struct{}),
	}

	return agent, nil
}

func (a *Agent) Start() error {
	log.Info("agent: Serf agent starting")

	// Create serf first
	serf, err := serf.Create(a.conf)
	if err != nil {
		return fmt.Errorf("Error creating Serf: %s", err)
	}
	a.serf = serf

	return nil
}

// Leave prepares for a graceful shutdown of the agent and its processes
func (a *Agent) Leave() error {
	if a.serf == nil {
		return nil
	}

	log.Info("agent: requesting graceful leave from Serf")
	return a.serf.Leave()
}

// Shutdown closes this agent and all of its processes. Should be preceded
// by a Leave for a graceful shutdown.
func (a *Agent) Shutdown() error {
	a.shutdownLock.Lock()
	defer a.shutdownLock.Unlock()

	if a.shutdown {
		return nil
	}

	if a.serf == nil {
		goto EXIT
	}

	log.Info("agent: requesting serf shutdown")
	if err := a.serf.Shutdown(); err != nil {
		return err
	}

EXIT:
	log.Info("agent: shutdown complete")
	a.shutdown = true
	close(a.shutdownCh)
	return nil
}

// ShutdownCh returns a channel that can be selected to wait
// for the agent to perform a shutdown.
func (a *Agent) ShutdownCh() <-chan struct{} {
	return a.shutdownCh
}

// Returns the Serf agent of the running Agent.
func (a *Agent) Serf() *serf.Serf {
	return a.serf
}

// Returns the Serf config of the running Agent.
func (a *Agent) SerfConfig() *serf.Config {
	return a.conf
}

// Join asks the Serf instance to join. See the Serf.Join function.
func (a *Agent) Join1(addr string) (n int, err error) {
	// a.logger.Printf("[INFO] agent: joining: %v replay: %v", addrs, replay)
	n, err = a.serf.Join([]string{addr}, true)
	if n > 0 {
		log.Info("agent: joined: %d nodes", n)
	}
	if err != nil {
		log.Warn("agent: error joining: %v", err)
	}
	return
}

// Join asks the Serf instance to join. See the Serf.Join function.
func (a *Agent) Join(addrs []string, replay bool) (n int, err error) {
	log.Info("agent: joining: %v replay: %v", addrs, replay)
	ignoreOld := !replay
	n, err = a.serf.Join(addrs, ignoreOld)
	if n > 0 {
		log.Info("agent: joined: %d nodes", n)
	}
	if err != nil {
		log.Warn("agent: error joining: %v", err)
	}
	return
}

// ForceLeave is used to eject a failed node from the cluster
func (a *Agent) ForceLeave(node string) error {
	log.Info("agent: Force leaving node: %s", node)
	err := a.serf.RemoveFailedNode(node)
	if err != nil {
		log.Info("agent: failed to remove node: %v", err)
	}
	return err
}

func (a *Agent) IsManager() {
	return a.isManager
}

// eventLoop listens to events from Serf and fans out to event handlers
func (a *Agent) eventLoop() {
	serfShutdownCh := a.serf.ShutdownCh()
	for {
		select {
		case e := <-a.eventCh:
			if a.IsManager() {
				if e.EventType() == serf.EventMemberJoin {
					// collect
					for _, m := range a.serf.Members() {
						log.Infof("Node(%s) = DOCKER_HOST(%s)", m.Name, m.Tags["DOCKER_HOST"])
					}
					// register node
				}
			}

		case <-serfShutdownCh:
			log.Warn("agent: Serf shutdown detected, quitting")
			a.Shutdown()
			return

		case <-a.shutdownCh:
			return

		}
	}
}

var running int = 0;

func checkPort(port uint16) string {
	ch := make(chan string, 1)
    for _, ip := range ips {
        for running >= 50 {
            time.Sleep(1 * time.Second)
        }
        running++
        go check(ip, port, ch)

    	if result := <-ch; result != "" {
    		return result
    	}

    }

    for running != 0 {
        time.Sleep(1 * time.Second)
    }
    return ""
}

func check(ip string, port uint16, ch chan string) {
    connection, err := net.DialTimeout("tcp",
    	ip + ":" + fmt.Sprintf("%d", port),
    	2 * time.Second)
	if err == nil {
		connection.Close()
		ch <- ip
    }
    running--
}

func scanPort(subnet string, port int) {

}

func main() {
	agent, err := Create()
	if err != nil {
		os.Exit(1)
	}

	ip := "" // scanPort("192.168.0.0/24", 3388)
	if ip == "" {
		agent.Start()
	} else {
		agent.Join1(ip)
	}

	for {
		agent.eventLoop()
	}
}

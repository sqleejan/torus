package server

import (
	"io"
	"math/rand"
	"time"

	"github.com/coreos/agro"
)

// Goroutine which watches for new rings and kicks off
// the rebalance dance.
func (d *distributor) ringWatcher(closer chan struct{}) {
	ch := make(chan agro.Ring)
	d.srv.mds.SubscribeNewRings(ch)
exit:
	for {
		select {
		case <-closer:
			d.srv.mds.UnsubscribeNewRings(ch)
			close(ch)
			break exit
		case newring, ok := <-ch:
			if ok {
				if newring.Version() == d.ring.Version() {
					// No problem. We're seeing the same ring.
					continue
				}
				if newring.Version() != d.ring.Version()+1 {
					panic("replacing old ring with ring in the far future!")
				}
				d.mut.Lock()
				d.ring = newring
				d.mut.Unlock()
			} else {
				break exit
			}
		}
	}
}

func (d *distributor) rebalanceTicker(closer chan struct{}) {
	n := 0
	total := 0
	time.Sleep(time.Duration(250+rand.Intn(250)) * time.Millisecond)
exit:
	for {
		timeout := 10 * time.Duration(n+1) * time.Millisecond
		select {
		case <-closer:
			break exit
		case <-time.After(timeout):
			written, err := d.rebalancer.Tick()
			total += written
			d.srv.peerInfo.LastRebalanceBlocks = uint64(total)
			if err == io.EOF {
				// Good job, sleep well, I'll most likely rebalance you in the morning.
				d.srv.peerInfo.LastRebalanceFinish = time.Now().UnixNano()
				total = 0
				time.Sleep(time.Duration(rand.Intn(5)) * time.Second)
				continue exit
			} else if err != nil {
				// This is usually really bad
				clog.Error(err)
			}
			n = written
		}
	}
}

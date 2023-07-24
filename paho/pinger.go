package paho

import (
	"fmt"
	"time"

	"github.com/eclipse/paho.golang/packets"
)

type Pinger func(
	receive <-chan *packets.Pingresp, 
	send chan<- *packets.ControlPacket, 
	stop <-chan struct{},
	error chan<- error,
	pt time.Duration,
	debug Logger,
);

func DefaultPinger(
	receive <-chan *packets.Pingresp, 
	send chan<- *packets.ControlPacket, 
	stop <-chan struct{},
	error chan<- error,
	pt time.Duration,
	debug Logger,
) {
	debug.Println("Starting pinger")
	sendNextPing := time.After(pt)
	pingExpired := make(<-chan time.Time)

	for {
		select {
		case <- stop:
			debug.Println("Pinger stopped")
			return
		case <- sendNextPing:
		    debug.Println("Sending next ping")
			go func() {
				send <- packets.NewControlPacket(packets.PINGREQ)
			}()
			sendNextPing = nil
			pingExpired = time.After(pt / 2)
		case <- pingExpired:
			debug.Println("Pinger expired")
			error <- fmt.Errorf("ping resp timed out")
			return
		case <- receive:
			debug.Println("Pingresp received.  Resetting.")
			sendNextPing = time.After(pt)
			pingExpired = nil
		}
	}
}


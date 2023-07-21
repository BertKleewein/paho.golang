package paho

import (
	"fmt"
	"time"

	"github.com/eclipse/paho.golang/packets"
)


func StartPinger(
	cPingresp chan *packets.Pingresp, 
	cOutgoingPackets chan *packets.ControlPacket, 
	cStop chan struct{},
	cError chan error,
	pt time.Duration,
	debug Logger,
) {
	debug.Println("Starting pinger")
	sendNextPing := time.After(pt)
	pingExpired := make(<-chan time.Time)

	for {
		select {
		case <- cStop:
			debug.Println("Pinger stopped")
			return
		case <- sendNextPing:
		    debug.Println("Sending next ping")
			go func() {
				cOutgoingPackets <- packets.NewControlPacket(packets.PINGREQ)
			}()
			sendNextPing = nil
			pingExpired = time.After(pt / 2)
		case <- pingExpired:
			debug.Println("Pinger expired")
			cError <- fmt.Errorf("ping resp timed out")
			return
		case <- cPingresp:
			debug.Println("Pingresp received.  Resetting.")
			sendNextPing = time.After(pt)
			pingExpired = nil
		}
	}
}


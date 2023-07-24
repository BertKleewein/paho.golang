package paho

import (
	"testing"
	"time"
	"github.com/eclipse/paho.golang/packets"
	"github.com/stretchr/testify/assert"
)

const keepalive time.Duration = 20 * time.Millisecond

func TestPingerPingHandlerTimeout(t *testing.T) {

	cOutgoingPackets := make(chan *packets.ControlPacket)
	cPingresp := make(chan *packets.Pingresp)
	cStop := make(chan struct{})
	cError := make(chan error)
	debug := NOOPLogger{}
	// debug := log.New(os.Stderr, "PINGTIMEOUT: ", log.LstdFlags)

	go DefaultPinger(cPingresp, cOutgoingPackets, cStop, cError, keepalive, debug)
	defer close(cStop)
	
	select {
	case <-time.After(keepalive * 3):
		t.Error("pingHandler did not timeout in time")
	case err := <-cError:
		assert.EqualError(t, err, "ping resp timed out")
	}
}

func TestPingerPingHandlerSuccess(t *testing.T) {
	cOutgoingPackets := make(chan *packets.ControlPacket)
	cPingresp := make(chan *packets.Pingresp)
	cStop := make(chan struct{})
	cError := make(chan error)
	debug := NOOPLogger{}
	// debug = log.New(os.Stderr, "PINGSUCCESS: ", log.LstdFlags)


	go DefaultPinger(cPingresp, cOutgoingPackets, cStop, cError, keepalive, debug)
	defer close(cStop)

	select {
	case p := <- cOutgoingPackets:
		_, success := p.Content.(*packets.Pingreq)
		assert.True(t, success)
		cPingresp <- packets.NewControlPacket(packets.PINGRESP).Content.(*packets.Pingresp)
	case <-time.After(keepalive * 3):
		// Pass
	case <-cError:
		t.Error("pingFailHandler was called when it should not have been")
	}
}

package paho

import (
	"testing"
	"time"
	"github.com/eclipse/paho.golang/packets"
	"github.com/stretchr/testify/assert"
)

func TestPingerPingHandlerTimeout(t *testing.T) {

	pfhValues := make(chan error)
	pfh := func(e error) {
		pfhValues <- e
	}

	cOutgoingPackets := make(chan *packets.ControlPacket)
	cPingresp := make(chan *packets.Pingresp)
	pingHandler := DefaultPingerWithCustomFailHandler(pfh)
	// pingHandler.SetDebug(log.New(os.Stderr, "PINGTIMEOUT: ", log.LstdFlags))

	go func() {
		pingHandler.Start(cPingresp, cOutgoingPackets, time.Second)
	}()
	defer pingHandler.Stop()
	

	select {
	case <-time.After(time.Second * 3):
		t.Error("pingHandler did not timeout in time")
	case err := <-pfhValues:
		assert.EqualError(t, err, "ping resp timed out")
	}
}

func TestPingerPingHandlerSuccess(t *testing.T) {

	pfhValues := make(chan error)
	pfh := func(e error) {
		pfhValues <- e
	}

	cOutgoingPackets := make(chan *packets.ControlPacket)
	cPingresp := make(chan *packets.Pingresp)
	pingHandler := DefaultPingerWithCustomFailHandler(pfh)
	// pingHandler.SetDebug(log.New(os.Stderr, "PINGSUCCESS: ", log.LstdFlags))


	go func() {
		pingHandler.Start(cPingresp, cOutgoingPackets, time.Second*1)
	}()
	defer pingHandler.Stop()

	select {
	case p := <- cOutgoingPackets:
		_, success := p.Content.(*packets.Pingreq)
		assert.True(t, success)
		cPingresp <- packets.NewControlPacket(packets.PINGRESP).Content.(*packets.Pingresp)
	case <-time.After(time.Second * 3):
		// Pass
	case <-pfhValues:
		t.Error("pingFailHandler was called when it should not have been")
	}
}

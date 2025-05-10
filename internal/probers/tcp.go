package probers

import (
	"fmt"
	"net"
	"time"

	log "github.com/sirupsen/logrus"
)

// TCP struct holds information for Tcp probers and
// implements Prober interface.
type TCP struct {
	ProberConfig
}

// NewTCP is the constructor for each Tcp struct.
func NewTCP(c ProberConfig) *TCP {
	return &TCP{c}
}

// String is used when we want to print info about Tcp prober.
func (t *TCP) String() string {
	return fmt.Sprintf("TCP Prober Endpoint: %s - Interval: %v - Tag: %s - Retries: %d", t.endpoint, t.interval, t.tag, t.retries)
}

// Run is responsible for holding the logic of running the probing
// TCP measurements.
func (t *TCP) Run() {
	defer t.wg.Done()

	if t.isOneOff {
		t.runOneOff()
	} else {
		t.runInterval()
	}
}

// Stop sends a message to TCP's exit channel when it's time to stop.
func (t *TCP) Stop() {
	log.Debugf("Prober: %s will stop now", t)
	t.exit <- true
}

// runOneOff runs TCP probing measurement once with
// a number of retries and then exits.
func (t *TCP) runOneOff() {
	var isSuccess bool

	var err error

	log.Infof("Starting TCP (OneOff) prober for %s", t)

	loop := true
	for i := 0; i < t.retries && loop; i++ {
		select {
		case <-t.exit:
			log.Infof("TCP (OneOff): %s got message in exit channel, exiting", t)

			return
		default:
			log.Debugf("TCP (OneOff) for %s starts new trace probe", t)

			err = t.dial()
			if err == nil {
				isSuccess = true
				loop = false
			}
		}
	}

	t.promC.UpdateRequestsCounter(t.endpoint, "tcp", t.tag, "")

	if !isSuccess {
		log.Errorf("TCP (OneOff) prober of %s error: %v", t, err)
		t.promC.UpdateErrorsCounter(t.endpoint, "tcp", t.tag, err.Error())
	}
}

// runInterval starts a loop with a ticker that run the TCP
// probing measurements in the workers interval
// probing measurements in the workers interval.
func (t *TCP) runInterval() {
	ticker := time.NewTicker(t.interval)
	log.Infof("Starting TCP for %s", t)

	for {
		select {
		case <-t.exit:
			log.Infof("TCP: %s got message in exit channel, exiting", t)

			return
		case <-ticker.C:
			log.Debugf("TCP for %s starts new trace probe", t)
			err := t.dial()
			t.promC.UpdateRequestsCounter(t.endpoint, "tcp", t.tag, "")

			if err != nil {
				log.Errorf("TCP prober of %s error: %v", t, err)
				t.promC.UpdateErrorsCounter(t.endpoint, "tcp", t.tag, err.Error())
			}
		}
	}
}

func (t *TCP) dial() error {
	conn, err := net.Dial("tcp", t.endpoint)
	if err != nil {
		return err
	}

	defer func() {
		err = conn.Close()
	}()

	return nil
}


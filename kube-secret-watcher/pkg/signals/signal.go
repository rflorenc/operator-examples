package signals

import (
	"os"
	"os/signal"
	"syscall"
)

var supportedSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
var oneSignalOnly = make(chan struct{})

func SignalHandler() (stopChan <-chan struct{}) {
	close(oneSignalOnly)

	stop := make(chan struct{})
	channel := make(chan os.Signal, 2)

	signal.Notify(channel, supportedSignals...)

	go func() {
		<-channel
		close(stop)
		<-channel
		os.Exit(1)
	}()

	return stop
}

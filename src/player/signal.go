/* signal.go
 *
 * Signal Handlers
 */

package main

import (
	"fmt"
	o "orchestra"
	"os"
	"os/signal"
	"syscall"
)

// handle the signals.  By default, we ignore everything, but the
// three terminal signals, HUP, INT, TERM, we want to explicitly
// handle.
func signalHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	for {
		sig := <-c

		ux, ok := sig.(syscall.Signal)
		if !ok {
			o.Warn("Couldn't handle signal %s, Coercion failed", sig)
			continue
		}

		switch ux {
		case syscall.SIGHUP:
			o.Warn("Reloading Configuration")
			reloadScores <- 1
		case syscall.SIGINT:
			fmt.Fprintln(os.Stderr, "Interrupt Received - Terminating")
			//FIXME: Gentle Shutdown
			os.Exit(1)
		case syscall.SIGTERM:
			fmt.Fprintln(os.Stderr, "Terminate Received - Terminating")
			//FIXME: Gentle Shutdown
			os.Exit(2)
		}
	}

}

func init() {
	go signalHandler()
}

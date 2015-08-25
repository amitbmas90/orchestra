/* conductor.go
*/

package main

import (
	"errors"
	"flag"
	o	"orchestra"
)

var (
	ConfigFile = flag.String("config-file", "/etc/orchestra/conductor.conf", "File containing the conductor configuration")
	DontVerifyPeer		= flag.Bool("dont-verify-peer", false, "Ignore TLS verification for the peer")
	// this is used a lot for marshalling. I just can't stuff it
	// anywhere else.
	InvalidValueError = errors.New("Invalid value")
)


func main() {
	o.SetLogName("conductor")

	// parse command line options.
	flag.Parse()

	// Start the client registry - configuration parsing will block indefinately
	// if the registry listener isn't working
	StartRegistry()

	// do an initial configuration load - must happen before
	// MakeSpoolDir and InitDispatch()
	ConfigLoad()

	// Build the Spool Tree if necessary
	MakeSpoolDir()

	// Load any old state we had.
	LoadState()
	defer SaveState()

	// start the master dispatch system
	InitDispatch()

	// start the status listener
	StartHTTP()
	// start the audience listener
	StartAudienceSock()
	// service TLS requests.
	ServiceRequests()
}

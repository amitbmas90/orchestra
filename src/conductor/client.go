/* client.go
 *
 * Client Handling
 */

package main

import (
	"crypto/tls"
	"net"
	o "orchestra"
	"time"
)

const (
	KeepaliveDelay   = 200 * time.Second
	RetryDelay       = 10 * time.Second // Must be smaller than the keepalive to avoid channel race.
	OutputQueueDepth = 10               // This needs to be large enough that we don't deadlock on ourself.
)

type ClientInfo struct {
	Player       string
	PktOutQ      chan *o.WirePkt
	PktInQ       chan *o.WirePkt
	abortQ       chan int
	TaskQ        chan *TaskRequest
	connection   net.Conn
	pendingTasks map[uint64]*TaskRequest
}

func NewClientInfo() (client *ClientInfo) {
	client = new(ClientInfo)
	client.abortQ = make(chan int, 2)
	client.PktOutQ = make(chan *o.WirePkt, OutputQueueDepth)
	client.PktInQ = make(chan *o.WirePkt)
	client.TaskQ = make(chan *TaskRequest)

	return client
}

func (client *ClientInfo) Abort() {
	PlayerDied(client)
	reg := ClientGet(client.Player)
	if reg != nil {
		reg.Disassociate()
	}
	client.abortQ <- 1
}

func (client *ClientInfo) Name() (name string) {
	if client.Player == "" {
		return "UNK:" + client.connection.RemoteAddr().String()
	}
	return client.Player
}

// Must not be used from inside of handlers.
func (client *ClientInfo) Send(p *o.WirePkt) {
	client.PktOutQ <- p
}

// Can only be used form inside of handlers and the main client loop.
func (client *ClientInfo) sendNow(p *o.WirePkt) {
	_, err := p.Send(client.connection)
	if err != nil {
		o.Warn("Client %s: error sending packet: %s", client.Name(), err)
		client.Abort()
	}
}

func (client *ClientInfo) SendTask(task *TaskRequest) {
	tr := task.Encode()
	p, err := o.Encode(tr)
	o.MightFail(err, "Couldn't encode task for client")
	client.Send(p)
	task.RetryTime = time.Now().Add(RetryDelay)
}

func (client *ClientInfo) GotTask(task *TaskRequest) {
	/* first up, look at the task state */
	switch task.State {
	case TASK_QUEUED:
		fallthrough
	case TASK_PENDINGRESULT:
		/* this is a new task.  We should send it straight */
		task.Player = client.Player
		task.State = TASK_PENDINGRESULT
		client.pendingTasks[task.job.Id] = task
		client.SendTask(task)
		// request a update to the spool so the PENDING flag is stored.
		JobWriteUpdate(task.job.Id)
	case TASK_FINISHED:
		/* discard.  We don't care about tasks that are done. */
	}
}

// reset the task state so it can be requeued.
func CleanTask(task *TaskRequest) {
	task.State = TASK_QUEUED
	task.Player = ""
}

// this merges the state from the registry record into the client it's called against.
// it also copies back the active communication channels to the registry record.
func (client *ClientInfo) MergeState(regrecord *ClientInfo) {
	client.Player = regrecord.Player
	client.pendingTasks = regrecord.pendingTasks

	regrecord.TaskQ = client.TaskQ
	regrecord.abortQ = client.abortQ
	regrecord.PktOutQ = client.PktOutQ
	regrecord.PktInQ = client.PktInQ
	regrecord.connection = client.connection
}

// Sever the connection state from the client (used against registry records only)
func (client *ClientInfo) Disassociate() {
	client.TaskQ = nil
	client.abortQ = nil
	client.PktInQ = nil
	client.PktOutQ = nil
	client.connection = nil
}

func handleNop(client *ClientInfo, message interface{}) {
	o.Debug("Client %s: NOP received", client.Name())
}

func handleIdentify(client *ClientInfo, message interface{}) {
	if client.Player != "" {
		o.Warn("Client %s: tried to reintroduce itself", client.Name())
		client.Abort()
		return
	}
	ic, _ := message.(*o.IdentifyClient)
	o.Info("Client %s: identified itself as \"%s\"", client.Name(), *ic.Hostname)
	client.Player = *ic.Hostname
	if !HostAuthorised(client.Player) {
		o.Warn("Client %s: not authorised", client.Name())
		client.Abort()
		return
	}

	/* if we're TLS, verify the client's certificate given the name it used */
	tlsc, ok := client.connection.(*tls.Conn)
	if ok && !*DontVerifyPeer {
		cs := tlsc.ConnectionState()
		if cs.PeerCertificates == nil || cs.PeerCertificates[0] == nil {
			o.Warn("Client %s: peer didn't provide a certificate", client.Name())
			client.Abort()
			return
		}
		err := cs.PeerCertificates[0].VerifyHostname(client.Player)
		if err != nil {
			o.Warn("Client %s: couldn't be identified: %s", client.Name(), err)
			client.Abort()
			return
		}
	}
	reg := ClientGet(client.Player)
	if nil == reg {
		o.Warn("Client %s: couldn't register", client.Name())
		client.Abort()
		return
	}
	client.MergeState(reg)
}

func handleReadyForTask(client *ClientInfo, message interface{}) {
	PlayerWaitingForJob(client)
}

func handleIllegal(client *ClientInfo, message interface{}) {
	o.Warn("Client %s: sent illegal message")
	client.Abort()
}

func handleResult(client *ClientInfo, message interface{}) {
	jr, _ := message.(*o.ProtoTaskResponse)
	r := ResponseFromProto(jr)
	// at this point in time, we only care about terminal
	// condition codes.  a Job that isn't finished is just
	// prodding us back to let us know it lives.
	if r.IsFinished() {
		job := JobGet(r.id)
		if nil == job {
			o.Warn("Client %s: NAKing job%d, couldn't find job data", client.Name(), r.id)
			nack := o.MakeNack(r.id)
			client.sendNow(nack)
		} else {
			job := JobGet(r.id)
			if job != nil {
				/* if the job exists, Ack it. */
				ack := o.MakeAck(r.id)
				client.sendNow(ack)
			}
			// now, we only accept the results if we were
			// expecting the results (ie: it was pending)
			// and expunge the task information from the
			// pending list so we stop bugging the client for it.
			task, exists := client.pendingTasks[r.id]
			if exists {
				// store the result.
				if !JobAddResult(client.Player, r) {
					o.Assert("Couldn't add result for pending task")
				}

				// next, work out if the job is a retryable failure or not
				var didretry bool

				if r.DidFail() {
					o.Info("Client %s: reported failure for job%d", client.Name(), r.id)
					if r.CanRetry() {
						job := JobGet(r.id)
						if job.Scope == SCOPE_ONEOF {
							// right, we're finally deep enough to work out what's going on!
							JobDisqualifyPlayer(r.id, client.Player)
							if len(job.Players) >= 1 {
								// still players left we can try?  then go for it!
								CleanTask(task)
								DispatchTask(task)
								didretry = true
							}
						}
					}
				}
				if !didretry {
					// if we didn't retry, the task needs to be marked as finished.
					task.State = TASK_FINISHED
				}
				// update the job state.
				JobReviewState(r.id)

				delete(client.pendingTasks, r.id)
			}
		}
	}
}

var dispatcher = map[uint8]func(*ClientInfo, interface{}){
	o.TypeNop:            handleNop,
	o.TypeIdentifyClient: handleIdentify,
	o.TypeReadyForTask:   handleReadyForTask,
	o.TypeTaskResponse:   handleResult,
	/* C->P only messages, should never appear on the wire. */
	o.TypeTaskRequest: handleIllegal,
}

var loopFudge time.Duration = 10 * time.Millisecond /* 10 ms should be enough fudgefactor */
func clientLogic(client *ClientInfo) {
	loop := true
	for loop {
		var retryWait <-chan time.Time
		var retryTask *TaskRequest
		if client.Player != "" {
			var waitTime, now time.Time
			cleanPass := false
			attempts := 0
			for !cleanPass && attempts < 10 {
				/* reset our state for the pass */
				waitTime = time.Time{}
				retryTask = nil
				attempts++
				cleanPass = true
				now = time.Now().Add(loopFudge)
				// if the client is correctly associated,
				// evaluate all jobs for outstanding retries,
				// and work out when our next retry is due.
				for _, v := range client.pendingTasks {
					if v.RetryTime.Before(now) {
						client.SendTask(v)
						cleanPass = false
					} else {
						if waitTime == (time.Time{}) || v.RetryTime.Before(waitTime) {
							retryTask = v
							waitTime = v.RetryTime
						}
					}
				}
			}
			if attempts > 10 {
				o.Fail("Couldn't find next timeout without restarting excessively")
			}
			if retryTask != nil {
				retryWait = time.After(waitTime.Sub(time.Now()))
			}
		}
		select {
		case <-retryWait:
			client.SendTask(retryTask)
		case p := <-client.PktInQ:
			/* we've received a packet.  do something with it. */
			if client.Player == "" && p.Type != o.TypeIdentifyClient {
				o.Warn("Client %s: didn't identify self,- got type %d instead", client.Name(), p.Type)
				client.Abort()
				break
			}
			var upkt interface{}
			if p.Length > 0 {
				var err error

				upkt, err = p.Decode()
				if err != nil {
					o.Warn("Client %s: error unmarshalling message: %s", client.Name(), err)
					client.Abort()
					break
				}
			}
			handler, exists := dispatcher[p.Type]
			if exists {
				handler(client, upkt)
			} else {
				o.Warn("Client %s: Unhandled packet type: %d", client.Name(), p.Type)
			}
		case p := <-client.PktOutQ:
			if p != nil {
				client.sendNow(p)
			}
		case t := <-client.TaskQ:
			client.GotTask(t)
		case <-client.abortQ:
			loop = false
		case <-time.After(KeepaliveDelay):
			p := o.MakeNop()
			_, err := p.Send(client.connection)
			if err != nil {
				o.Warn("Client %s: error sending packet: %s", client.Name(), err)
				client.Abort()
			}
		}
	}
	client.connection.Close()
}

func clientReceiver(client *ClientInfo) {
	conn := client.connection

	loop := true
	for loop {
		pkt, err := o.Receive(conn)
		if nil != err {
			o.Warn("Client %s: error receiving packet: %s", conn.RemoteAddr().String(), err)
			client.Abort()
			client.connection.Close()
			loop = false
		} else {
			client.PktInQ <- pkt
		}
	}
}

/* The Main Server loop calls this method to hand off connections to us */
func HandleConnection(conn net.Conn) {
	/* this is a temporary client info, we substitute it for the real
	 * one once we ID the connection correctly */
	c := NewClientInfo()
	c.connection = conn
	go clientReceiver(c)
	go clientLogic(c)
}

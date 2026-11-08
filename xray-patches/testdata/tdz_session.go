package session

// TDZWaitForMuxKey keeps admission leases until asynchronous outbound MUX
// sessions finish, rather than releasing a slot when enqueueing the session.
type TDZWaitForMuxKey struct{}

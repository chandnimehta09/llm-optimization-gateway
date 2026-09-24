package reliability

// TimeoutPhase represents a specific phase of a request that can have its own timeout.
type TimeoutPhase string

const (
	// PhaseConnection is the TCP connection establishment phase.
	PhaseConnection TimeoutPhase = "connection"
	// PhaseDNS is the DNS resolution phase.
	PhaseDNS TimeoutPhase = "dns"
	// PhaseTLS is the TLS handshake phase.
	PhaseTLS TimeoutPhase = "tls"
	// PhaseRead is the response body read phase.
	PhaseRead TimeoutPhase = "read"
	// PhaseWrite is the request body write phase.
	PhaseWrite TimeoutPhase = "write"
	// PhaseTotal is the total end-to-end request timeout.
	PhaseTotal TimeoutPhase = "total"
)

// String returns the string representation of a TimeoutPhase.
func (p TimeoutPhase) String() string {
	return string(p)
}

// AllPhases returns all defined timeout phases.
func AllPhases() []TimeoutPhase {
	return []TimeoutPhase{
		PhaseConnection,
		PhaseDNS,
		PhaseTLS,
		PhaseRead,
		PhaseWrite,
		PhaseTotal,
	}
}

// IsValid checks whether the phase is a known timeout phase.
func (p TimeoutPhase) IsValid() bool {
	switch p {
	case PhaseConnection, PhaseDNS, PhaseTLS, PhaseRead, PhaseWrite, PhaseTotal:
		return true
	default:
		return false
	}
}

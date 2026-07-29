package kicadfiles

// PreservationStrategy records how a reader will carry a KiCad construct
// through a read-modify-write transaction.
type PreservationStrategy string

const (
	PreservationFullyModeled PreservationStrategy = "fully_modeled"
	PreservationRaw          PreservationStrategy = "raw_preserved"
	PreservationUnsupported  PreservationStrategy = "unsupported_for_mutation"
)

// PreservationCapability is reader-produced evidence for one construct.
// Path is stable within the parsed file and Family is the KiCad node head.
type PreservationCapability struct {
	Path     string
	Family   string
	Strategy PreservationStrategy
	Reason   string
}

// HasUnsupportedPreservation reports whether an imported file contains a
// construct that the reader cannot prove safe across mutation.
func HasUnsupportedPreservation(capabilities []PreservationCapability) bool {
	for _, capability := range capabilities {
		if capability.Strategy == PreservationUnsupported {
			return true
		}
	}
	return false
}

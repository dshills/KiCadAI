package schematicassistant

import "fmt"

// StageError identifies the first blocking stage without parsing human messages.
type StageError struct {
	Stage string
	Cause error
}

func (e *StageError) Error() string { return fmt.Sprintf("%s: %v", e.Stage, e.Cause) }
func (e *StageError) Unwrap() error { return e.Cause }

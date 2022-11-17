package utils

// ErrNonRestorableUsage indicates non restoreable usage error
// on any error system tries to restore users usage counter
// but on this error it does not
type ErrNonRestorableUsage struct {
	err error
}

// NewErrNonRestorableUsage creates new error
func NewErrNonRestorableUsage(err error) error {
	return &ErrNonRestorableUsage{err: err}
}

func (e *ErrNonRestorableUsage) Error() string {
	return "non restorable usage error: " + e.err.Error()
}

func (e *ErrNonRestorableUsage) Unwrap() error {
	return e.err
}

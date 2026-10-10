package memory

import (
	"database/sql"
	"errors"
	"fmt"
)

// rollbackTx rolls back tx and joins any real rollback failure onto err.
// sql.ErrTxDone after a successful commit is ignored (not a failure).
func rollbackTx(tx *sql.Tx, err error) error {
	if tx == nil {
		return err
	}
	rbErr := tx.Rollback()
	return joinRollbackError(err, rbErr)
}

// joinRollbackError preserves the original operation error and attaches a
// rollback failure when present. ErrTxDone is not treated as a failure.
func joinRollbackError(err, rbErr error) error {
	if rbErr == nil || errors.Is(rbErr, sql.ErrTxDone) {
		return err
	}
	rb := fmt.Errorf("memory: rollback: %w", rbErr)
	if err == nil {
		return rb
	}
	return errors.Join(err, rb)
}

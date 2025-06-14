// SPDX-License-Identifier: Apache-2.0

package migrations

import (
	"context"

	"github.com/xataio/pgroll/pkg/db"
	"github.com/xataio/pgroll/pkg/schema"
)

var (
	_ Operation  = (*OpRawSQL)(nil)
	_ Createable = (*OpRawSQL)(nil)
)

func (o *OpRawSQL) Start(ctx context.Context, l Logger, conn db.DB, latestSchema string, s *schema.Schema) (*schema.Table, error) {
	l.LogOperationStart(o)

	if o.OnComplete {
		return nil, nil
	}

	return nil, NewRawSQLAction(conn, o.Up).Execute(ctx)
}

func (o *OpRawSQL) Complete(ctx context.Context, l Logger, conn db.DB, s *schema.Schema) error {
	l.LogOperationComplete(o)

	if !o.OnComplete {
		return nil
	}

	return NewRawSQLAction(conn, o.Up).Execute(ctx)
}

func (o *OpRawSQL) Rollback(ctx context.Context, l Logger, conn db.DB, s *schema.Schema) error {
	l.LogOperationRollback(o)

	if o.Down == "" {
		return nil
	}

	return NewRawSQLAction(conn, o.Down).Execute(ctx)
}

func (o *OpRawSQL) Validate(ctx context.Context, s *schema.Schema) error {
	if o.Up == "" {
		return EmptyMigrationError{}
	}

	if o.OnComplete && o.Down != "" {
		return InvalidMigrationError{Reason: "down is not allowed with onComplete"}
	}

	return nil
}

// IsIsolated returns true if the operation is isolated and should be run with other operations.
func (o *OpRawSQL) IsIsolated() bool {
	return !o.OnComplete
}

func (o *OpRawSQL) RequiresSchemaRefresh() {}

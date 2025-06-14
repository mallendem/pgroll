// SPDX-License-Identifier: Apache-2.0

package migrations

import (
	"context"

	"github.com/xataio/pgroll/pkg/db"
	"github.com/xataio/pgroll/pkg/schema"
)

type OpSetDefault struct {
	Table   string  `json:"table"`
	Column  string  `json:"column"`
	Default *string `json:"default"`
	Up      string  `json:"up"`
	Down    string  `json:"down"`
}

var _ Operation = (*OpSetDefault)(nil)

func (o *OpSetDefault) Start(ctx context.Context, l Logger, conn db.DB, latestSchema string, s *schema.Schema) (*schema.Table, error) {
	l.LogOperationStart(o)

	table := s.GetTable(o.Table)
	if table == nil {
		return nil, TableDoesNotExistError{Name: o.Table}
	}
	column := table.GetColumn(o.Column)
	if column == nil {
		return nil, ColumnDoesNotExistError{Table: o.Table, Name: o.Column}
	}

	var err error
	if o.Default == nil {
		err = NewDropDefaultValueAction(conn, table.Name, column.Name).Execute(ctx)
		column.Default = nil
	} else {
		err = NewSetDefaultValueAction(conn, table.Name, column.Name, *o.Default).Execute(ctx)
		column.Default = o.Default
	}
	if err != nil {
		return nil, err
	}

	return table, nil
}

func (o *OpSetDefault) Complete(ctx context.Context, l Logger, conn db.DB, s *schema.Schema) error {
	l.LogOperationComplete(o)

	return nil
}

func (o *OpSetDefault) Rollback(ctx context.Context, l Logger, conn db.DB, s *schema.Schema) error {
	l.LogOperationRollback(o)

	return nil
}

func (o *OpSetDefault) Validate(ctx context.Context, s *schema.Schema) error {
	return nil
}

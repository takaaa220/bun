package migrate

import (
	"github.com/uptrace/bun/migrate/sqlschema"
	"github.com/uptrace/bun/schema"
)

// Operation encapsulates the request to change a database definition
// and knowns which operation can revert it.
type Operation interface {
	GetReverse() Operation
}

// CreateTable
type CreateTable struct {
	Schema string
	Name   string
	Model  interface{}
}

var _ Operation = (*CreateTable)(nil)

func (op *CreateTable) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.Schema,
		Table:  op.Name,
	}
}

func (op *CreateTable) GetReverse() Operation {
	return &DropTable{
		Schema: op.Schema,
		Name:   op.Name,
	}
}

type DropTable struct {
	Schema string
	Name   string
}

var _ Operation = (*DropTable)(nil)

func (op *DropTable) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.Schema,
		Table:  op.Name,
	}
}

func (op *DropTable) DependsOn(another Operation) bool {
	d, ok := another.(*DropConstraint)
	//
	return ok && ((d.FK.From.Schema == op.Schema && d.FK.From.Table == op.Name) ||
		(d.FK.To.Schema == op.Schema && d.FK.To.Table == op.Name))
}

// GetReverse for a DropTable returns a no-op migration. Logically, CreateTable is the reverse,
// but DropTable does not have the table's definition to create one.
//
// TODO: we can fetch table definitions for deleted tables
// from the database engine and execute them as a raw query.
func (op *DropTable) GetReverse() Operation {
	return &noop{}
}

type RenameTable struct {
	Schema  string
	OldName string
	NewName string
}

var _ Operation = (*RenameTable)(nil)
var _ sqlschema.Operation = (*RenameTable)(nil)

func (op *RenameTable) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.Schema,
		Table:  op.OldName,
	}
}

func (op *RenameTable) GetReverse() Operation {
	return &RenameTable{
		Schema:  op.Schema,
		OldName: op.NewName,
		NewName: op.OldName,
	}
}

// RenameColumn.
type RenameColumn struct {
	Schema  string
	Table   string
	OldName string
	NewName string
}

var _ Operation = (*RenameColumn)(nil)
var _ sqlschema.Operation = (*RenameColumn)(nil)

func (op *RenameColumn) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.Schema,
		Table:  op.Table,
	}
}

func (op *RenameColumn) GetReverse() Operation {
	return &RenameColumn{
		Schema:  op.Schema,
		Table:   op.Table,
		OldName: op.NewName,
		NewName: op.OldName,
	}
}

func (op *RenameColumn) DependsOn(another Operation) bool {
	rt, ok := another.(*RenameTable)
	return ok && rt.Schema == op.Schema && rt.NewName == op.Table
}

type AddColumn struct {
	Schema string
	Table  string
	Column string
	ColDef sqlschema.Column
}

var _ Operation = (*AddColumn)(nil)
var _ sqlschema.Operation = (*AddColumn)(nil)

func (op *AddColumn) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.Schema,
		Table:  op.Table,
	}
}

func (op *AddColumn) GetReverse() Operation {
	return &DropColumn{
		Schema: op.Schema,
		Table:  op.Table,
		Column: op.Column,
	}
}

type DropColumn struct {
	Schema string
	Table  string
	Column string
	ColDef sqlschema.Column
}

var _ Operation = (*DropColumn)(nil)
var _ sqlschema.Operation = (*DropColumn)(nil)

func (op *DropColumn) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.Schema,
		Table:  op.Table,
	}
}

func (op *DropColumn) GetReverse() Operation {
	return &AddColumn{
		Schema: op.Schema,
		Table:  op.Table,
		Column: op.Column,
		ColDef: op.ColDef,
	}
}

func (op *DropColumn) DependsOn(another Operation) bool {
	// TODO: refactor
	if dc, ok := another.(*DropConstraint); ok {
		var fCol bool
		fCols := dc.FK.From.Column.Split()
		for _, c := range fCols {
			if c == op.Column {
				fCol = true
				break
			}
		}

		var tCol bool
		tCols := dc.FK.To.Column.Split()
		for _, c := range tCols {
			if c == op.Column {
				tCol = true
				break
			}
		}

		return (dc.FK.From.Schema == op.Schema && dc.FK.From.Table == op.Table && fCol) ||
			(dc.FK.To.Schema == op.Schema && dc.FK.To.Table == op.Table && tCol)
	}
	return false
}

// RenameConstraint.
type RenameConstraint struct {
	FK      sqlschema.FK
	OldName string
	NewName string
}

var _ Operation = (*RenameConstraint)(nil)
var _ sqlschema.Operation = (*RenameConstraint)(nil)

func (op *RenameConstraint) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.FK.From.Schema,
		Table:  op.FK.From.Table,
	}
}

func (op *RenameConstraint) DependsOn(another Operation) bool {
	rt, ok := another.(*RenameTable)
	return ok && rt.Schema == op.FK.From.Schema && rt.NewName == op.FK.From.Table
}

func (op *RenameConstraint) GetReverse() Operation {
	return &RenameConstraint{
		FK:      op.FK,
		OldName: op.OldName,
		NewName: op.NewName,
	}
}

type AddForeignKey struct {
	FK             sqlschema.FK
	ConstraintName string
}

var _ Operation = (*AddForeignKey)(nil)
var _ sqlschema.Operation = (*AddForeignKey)(nil)

func (op *AddForeignKey) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.FK.From.Schema,
		Table:  op.FK.From.Table,
	}
}

func (op *AddForeignKey) DependsOn(another Operation) bool {
	switch another := another.(type) {
	case *RenameTable:
		// TODO: provide some sort of "DependsOn" method for FK
		return another.Schema == op.FK.From.Schema && another.NewName == op.FK.From.Table
	case *CreateTable:
		return (another.Schema == op.FK.To.Schema && another.Name == op.FK.To.Table) || // either it's the referencing one
			(another.Schema == op.FK.From.Schema && another.Name == op.FK.From.Table) // or the one being referenced
	}
	return false
}

func (op *AddForeignKey) GetReverse() Operation {
	return &DropConstraint{
		FK:             op.FK,
		ConstraintName: op.ConstraintName,
	}
}

// DropConstraint.
type DropConstraint struct {
	FK             sqlschema.FK
	ConstraintName string
}

var _ Operation = (*DropConstraint)(nil)
var _ sqlschema.Operation = (*DropConstraint)(nil)

func (op *DropConstraint) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.FK.From.Schema,
		Table:  op.FK.From.Table,
	}
}

func (op *DropConstraint) GetReverse() Operation {
	return &AddForeignKey{
		FK:             op.FK,
		ConstraintName: op.ConstraintName,
	}
}

// Change column type.
type ChangeColumnType struct {
	Schema string
	Table  string
	Column string
	From   sqlschema.Column
	To     sqlschema.Column
}

var _ Operation = (*ChangeColumnType)(nil)
var _ sqlschema.Operation = (*ChangeColumnType)(nil)

func (op *ChangeColumnType) GetReverse() Operation {
	return &ChangeColumnType{
		Schema: op.Schema,
		Table:  op.Table,
		Column: op.Column,
		From:   op.To,
		To:     op.From,
	}
}

func (op ChangeColumnType) FQN() schema.FQN {
	return schema.FQN{
		Schema: op.Schema,
		Table:  op.Table,
	}
}

// noop is a migration that doesn't change the schema.
type noop struct{}

var _ Operation = (*noop)(nil)

func (*noop) GetReverse() Operation { return &noop{} }

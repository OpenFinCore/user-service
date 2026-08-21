// Package repository provides one generic, filter-driven CRUD engine used
// by every table in the schema, instead of a hand-written function per
// query. Callers pick a model type T and describe what they want with a
// QueryOptions value (filters, preloads, ordering, paging); the same five
// functions (Create, Get, List, Update, Delete - plus Count/Exists) work
// for any table.
package repository

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Op is a whitelisted comparison operator usable in a Filter. Restricting
// filters to this set (rather than accepting raw SQL fragments) is what
// keeps the generic engine safe to drive from caller-supplied field names.
type Op string

const (
	OpEq      Op = "="
	OpNeq     Op = "!="
	OpGt      Op = ">"
	OpGte     Op = ">="
	OpLt      Op = "<"
	OpLte     Op = "<="
	OpLike    Op = "LIKE"
	OpILike   Op = "ILIKE"
	OpIn      Op = "IN"
	OpNotIn   Op = "NOT IN"
	OpIsNull  Op = "IS NULL"
	OpNotNull Op = "IS NOT NULL"
)

var allowedOps = map[Op]bool{
	OpEq: true, OpNeq: true, OpGt: true, OpGte: true, OpLt: true, OpLte: true,
	OpLike: true, OpILike: true, OpIn: true, OpNotIn: true, OpIsNull: true, OpNotNull: true,
}

// Filter is a single `column <op> value` predicate, ANDed together with the
// rest of the filters in a QueryOptions.
type Filter struct {
	Field string
	Op    Op
	Value interface{}
}

// Convenience constructors for the common cases.
func Eq(field string, value interface{}) Filter { return Filter{Field: field, Op: OpEq, Value: value} }
func Neq(field string, value interface{}) Filter {
	return Filter{Field: field, Op: OpNeq, Value: value}
}
func Gt(field string, value interface{}) Filter { return Filter{Field: field, Op: OpGt, Value: value} }
func Gte(field string, value interface{}) Filter {
	return Filter{Field: field, Op: OpGte, Value: value}
}
func Lt(field string, value interface{}) Filter { return Filter{Field: field, Op: OpLt, Value: value} }
func Lte(field string, value interface{}) Filter {
	return Filter{Field: field, Op: OpLte, Value: value}
}
func Like(field string, value string) Filter  { return Filter{Field: field, Op: OpLike, Value: value} }
func ILike(field string, value string) Filter { return Filter{Field: field, Op: OpILike, Value: value} }
func In(field string, values interface{}) Filter {
	return Filter{Field: field, Op: OpIn, Value: values}
}
func NotIn(field string, values interface{}) Filter {
	return Filter{Field: field, Op: OpNotIn, Value: values}
}
func IsNull(field string) Filter    { return Filter{Field: field, Op: OpIsNull} }
func IsNotNull(field string) Filter { return Filter{Field: field, Op: OpNotNull} }

// QueryOptions describes what to fetch/update/delete. The zero value means
// "no filters, no preloads, natural order" - it is never valid for Update
// or Delete, which require at least one filter as a safety rail against
// accidental table-wide writes.
type QueryOptions struct {
	Filters  []Filter
	Preloads []string
	OrderBy  string
	Limit    int
	Offset   int
	// Unscoped includes soft-deleted rows (models with a gorm.DeletedAt field).
	Unscoped bool
}

func isSafeIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '.':
		default:
			return false
		}
	}
	return true
}

func buildClause(f Filter) (string, error) {
	if !allowedOps[f.Op] {
		return "", fmt.Errorf("repository: unsupported filter operator %q", f.Op)
	}
	if !isSafeIdentifier(f.Field) {
		return "", fmt.Errorf("repository: unsafe filter field %q", f.Field)
	}
	switch f.Op {
	case OpIsNull, OpNotNull:
		return fmt.Sprintf("%s %s", f.Field, f.Op), nil
	case OpIn, OpNotIn:
		return fmt.Sprintf("%s %s (?)", f.Field, f.Op), nil
	default:
		return fmt.Sprintf("%s %s ?", f.Field, f.Op), nil
	}
}

func applyFilters(db *gorm.DB, filters []Filter) (*gorm.DB, error) {
	q := db
	for _, f := range filters {
		clause, err := buildClause(f)
		if err != nil {
			return nil, err
		}
		if f.Op == OpIsNull || f.Op == OpNotNull {
			q = q.Where(clause)
		} else {
			q = q.Where(clause, f.Value)
		}
	}
	return q, nil
}

func applyOptions(db *gorm.DB, opts QueryOptions) (*gorm.DB, error) {
	q := db
	if opts.Unscoped {
		q = q.Unscoped()
	}
	q, err := applyFilters(q, opts.Filters)
	if err != nil {
		return nil, err
	}
	for _, p := range opts.Preloads {
		q = q.Preload(p)
	}
	if opts.OrderBy != "" {
		q = q.Order(opts.OrderBy)
	}
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		q = q.Offset(opts.Offset)
	}
	return q, nil
}

// Create inserts a single new record.
func Create[T any](db *gorm.DB, entity *T) error {
	return db.Create(entity).Error
}

// CreateMany inserts a batch of new records in one statement.
func CreateMany[T any](db *gorm.DB, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}
	return db.Create(&entities).Error
}

// Get returns the first record matching opts, or (nil, nil) if none exists.
func Get[T any](db *gorm.DB, opts QueryOptions) (*T, error) {
	var result T
	q, err := applyOptions(db.Model(&result), opts)
	if err != nil {
		return nil, err
	}
	if err := q.First(&result).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

// List returns every record matching opts along with the total count of
// matching rows ignoring Limit/Offset (useful for pagination).
func List[T any](db *gorm.DB, opts QueryOptions) ([]T, int64, error) {
	var results []T

	countQ, err := applyOptions(db.Model(&results), QueryOptions{Filters: opts.Filters, Unscoped: opts.Unscoped})
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q, err := applyOptions(db.Model(&results), opts)
	if err != nil {
		return nil, 0, err
	}
	if err := q.Find(&results).Error; err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

// Update applies the given column updates to every record matching opts.
// opts must carry at least one filter - GORM refuses a WHERE-less update.
func Update[T any](db *gorm.DB, opts QueryOptions, updates map[string]interface{}) (int64, error) {
	var model T
	q, err := applyOptions(db.Model(&model), opts)
	if err != nil {
		return 0, err
	}
	res := q.Updates(updates)
	return res.RowsAffected, res.Error
}

// Save persists every field of entity (a full row replace), using its
// primary key.
func Save[T any](db *gorm.DB, entity *T) error {
	return db.Save(entity).Error
}

// Delete removes every record matching opts. If the model has a
// gorm.DeletedAt field this is a soft delete; otherwise it is permanent.
// opts must carry at least one filter.
func Delete[T any](db *gorm.DB, opts QueryOptions) (int64, error) {
	var model T
	q, err := applyOptions(db.Model(&model), opts)
	if err != nil {
		return 0, err
	}
	res := q.Delete(&model)
	return res.RowsAffected, res.Error
}

// HardDelete permanently removes every record matching opts, bypassing
// soft-delete even if the model has a gorm.DeletedAt field.
func HardDelete[T any](db *gorm.DB, opts QueryOptions) (int64, error) {
	var model T
	q, err := applyOptions(db.Unscoped().Model(&model), opts)
	if err != nil {
		return 0, err
	}
	res := q.Delete(&model)
	return res.RowsAffected, res.Error
}

// Count returns the number of records matching opts (Preloads/Limit/Offset
// are ignored).
func Count[T any](db *gorm.DB, opts QueryOptions) (int64, error) {
	var model T
	q, err := applyOptions(db.Model(&model), QueryOptions{Filters: opts.Filters, Unscoped: opts.Unscoped})
	if err != nil {
		return 0, err
	}
	var count int64
	err = q.Count(&count).Error
	return count, err
}

// Exists reports whether any record matches opts.
func Exists[T any](db *gorm.DB, opts QueryOptions) (bool, error) {
	count, err := Count[T](db, opts)
	return count > 0, err
}

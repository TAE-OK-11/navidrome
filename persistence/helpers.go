package persistence

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/squirrel"
)

type PostMapper interface {
	PostMapArgs(map[string]any) error
}

func toSQLArgs(rec any) (map[string]any, error) {
	value := reflect.ValueOf(rec)
	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("cannot map nil %T", rec)
		}
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct, got %T", rec)
	}

	fields := cachedSQLFields(value.Type())
	m := make(map[string]any, len(fields))
	for _, field := range fields {
		fieldValue, ok := valueByIndex(value, field.index)
		if !ok || field.omitEmpty && fieldValue.IsZero() {
			continue
		}

		var mapped any
		if field.asString {
			stringer, ok := fieldValue.Interface().(fmt.Stringer)
			if !ok {
				continue
			}
			mapped = stringer.String()
		} else {
			mapped = fieldValue.Interface()
		}

		switch t := mapped.(type) {
		case *time.Time:
			if t != nil {
				mapped = *t
			}
		case driver.Valuer:
			v, err := t.Value()
			if err != nil {
				return nil, err
			}
			mapped = v
		}
		m[field.name] = mapped
	}
	if r, ok := rec.(PostMapper); ok {
		err := r.PostMapArgs(m)
		if err != nil {
			return nil, err
		}
	}
	return m, nil
}

type sqlField struct {
	name      string
	index     []int
	omitEmpty bool
	asString  bool
}

var sqlFieldCache sync.Map

func cachedSQLFields(t reflect.Type) []sqlField {
	if cached, ok := sqlFieldCache.Load(t); ok {
		return cached.([]sqlField)
	}
	fields := collectSQLFields(t, nil)
	actual, _ := sqlFieldCache.LoadOrStore(t, fields)
	return actual.([]sqlField)
}

func collectSQLFields(t reflect.Type, parentIndex []int) []sqlField {
	fields := make([]sqlField, 0, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}

		tag := field.Tag.Get("structs")
		if tag == "-" {
			continue
		}
		name, options, _ := strings.Cut(tag, ",")
		index := make([]int, len(parentIndex)+1)
		copy(index, parentIndex)
		index[len(parentIndex)] = i

		if hasStructOption(options, "flatten") {
			fieldType := field.Type
			for fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}
			if fieldType.Kind() == reflect.Struct {
				fields = append(fields, collectSQLFields(fieldType, index)...)
			}
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields = append(fields, sqlField{
			name:      name,
			index:     index,
			omitEmpty: hasStructOption(options, "omitempty"),
			asString:  hasStructOption(options, "string"),
		})
	}
	return fields
}

func hasStructOption(options, target string) bool {
	for option := range strings.SplitSeq(options, ",") {
		if option == target {
			return true
		}
	}
	return false
}

func valueByIndex(value reflect.Value, index []int) (reflect.Value, bool) {
	for _, fieldIndex := range index {
		for value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct {
			return reflect.Value{}, false
		}
		value = value.Field(fieldIndex)
	}
	return value, value.IsValid() && value.CanInterface()
}

func toSnakeCase(str string) string {
	hasUpper := false
	hasNonASCII := false
	for i := range len(str) {
		c := str[i]
		hasUpper = hasUpper || c >= 'A' && c <= 'Z'
		hasNonASCII = hasNonASCII || c >= 0x80
	}
	if !hasUpper {
		if hasNonASCII {
			return strings.ToLower(str)
		}
		return str
	}

	var snake strings.Builder
	snake.Grow(len(str) + 4)
	for i := range len(str) {
		c := str[i]
		if i > 0 && c >= 'A' && c <= 'Z' {
			previous := str[i-1]
			nextIsLower := i+1 < len(str) && str[i+1] >= 'a' && str[i+1] <= 'z'
			previousIsLowerOrDigit := previous >= 'a' && previous <= 'z' || previous >= '0' && previous <= '9'
			if nextIsLower || previousIsLowerOrDigit {
				snake.WriteByte('_')
			}
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		snake.WriteByte(c)
	}
	result := snake.String()
	if hasNonASCII {
		return strings.ToLower(result)
	}
	return result
}

func toCamelCase(str string) string {
	first := -1
	for i := 0; i+1 < len(str); i++ {
		if str[i] == '_' && isASCIILetter(str[i+1]) {
			first = i
			break
		}
	}
	if first < 0 {
		return str
	}

	var camel strings.Builder
	camel.Grow(len(str) - 1)
	camel.WriteString(str[:first])
	for i := first; i < len(str); i++ {
		if str[i] == '_' && i+1 < len(str) && isASCIILetter(str[i+1]) {
			next := str[i+1]
			if next >= 'a' && next <= 'z' {
				next -= 'a' - 'A'
			}
			camel.WriteByte(next)
			i++
			continue
		}
		camel.WriteByte(str[i])
	}
	return camel.String()
}

func isASCIILetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func Exists(subTable string, cond squirrel.Sqlizer) existsCond {
	return existsCond{subTable: subTable, cond: cond, not: false}
}

func NotExists(subTable string, cond squirrel.Sqlizer) existsCond {
	return existsCond{subTable: subTable, cond: cond, not: true}
}

type existsCond struct {
	subTable string
	cond     squirrel.Sqlizer
	not      bool
}

func (e existsCond) ToSql() (string, []any, error) {
	sql, args, err := e.cond.ToSql()
	sql = fmt.Sprintf("exists (select 1 from %s where %s)", e.subTable, sql)
	if e.not {
		sql = "not " + sql
	}
	return sql, args, err
}

var sortOrderRegex = regexp.MustCompile(`order_([a-z_]+)`)

// Convert the order_* columns to an expression using sort_* columns. Example:
// sort_album_name -> (coalesce(nullif(sort_album_name,”),order_album_name) collate nocase)
// It finds order column names anywhere in the substring
func mapSortOrder(tableName, order string) string {
	order = strings.ToLower(order)
	repl := fmt.Sprintf("(coalesce(nullif(%[1]s.sort_$1,''),%[1]s.order_$1) collate nocase)", tableName)
	return sortOrderRegex.ReplaceAllString(order, repl)
}

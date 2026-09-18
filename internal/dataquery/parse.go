package dataquery

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/vrc/nimbus/internal/domain"
)

var (
	collectionRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	fieldRe      = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)
)

// ValidCollection returns true for safe collection names.
func ValidCollection(name string) bool {
	return collectionRe.MatchString(name)
}

// ValidField returns true for safe JSON field / column names.
func ValidField(name string) bool {
	return fieldRe.MatchString(name)
}

// Parse reads PostgREST-style query params from an HTTP request.
// Example: ?price=gte.10&name=like.*shirt*&order=price.desc&limit=20
func Parse(r *http.Request) (domain.DataQuery, error) {
	q := domain.DataQuery{Limit: 50}
	for key, vals := range r.URL.Query() {
		if len(vals) == 0 {
			continue
		}
		val := vals[0]
		switch key {
		case "limit":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return q, fmt.Errorf("%w: invalid limit", domain.ErrValidation)
			}
			if n > 100 {
				n = 100
			}
			q.Limit = n
		case "offset":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return q, fmt.Errorf("%w: invalid offset", domain.ErrValidation)
			}
			q.Offset = n
		case "order":
			parts := strings.Split(val, ".")
			field := parts[0]
			if !ValidField(field) && field != "created_at" && field != "updated_at" {
				return q, fmt.Errorf("%w: invalid order field", domain.ErrValidation)
			}
			q.OrderBy = field
			if len(parts) > 1 && strings.EqualFold(parts[1], "desc") {
				q.Desc = true
			}
		default:
			f, err := parseFilter(key, val)
			if err != nil {
				return q, err
			}
			q.Filters = append(q.Filters, f)
		}
	}
	return q, nil
}

func parseFilter(field, raw string) (domain.DataFilter, error) {
	if !ValidField(field) {
		return domain.DataFilter{}, fmt.Errorf("%w: invalid field %q", domain.ErrValidation, field)
	}
	op := "eq"
	value := raw
	if i := strings.Index(raw, "."); i > 0 {
		op = raw[:i]
		value = raw[i+1:]
	}
	switch op {
	case "eq", "gt", "gte", "lt", "lte", "like":
	default:
		return domain.DataFilter{}, fmt.Errorf("%w: unknown operator %q", domain.ErrValidation, op)
	}
	if op == "like" {
		value = strings.ReplaceAll(value, "*", "%")
	}
	return domain.DataFilter{Field: field, Op: op, Value: value}, nil
}

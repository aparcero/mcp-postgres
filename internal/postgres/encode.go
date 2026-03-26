package postgres

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/aparcero/mcp-postgres/internal/types"
)

func buildQueryColumns(typeMap *pgtype.Map, fields []pgconn.FieldDescription) []types.QueryColumn {
	seen := make(map[string]int, len(fields))
	columns := make([]types.QueryColumn, 0, len(fields))

	for index, field := range fields {
		sourceName := strings.TrimSpace(field.Name)
		if sourceName == "" {
			sourceName = fmt.Sprintf("column_%d", index+1)
		}

		seen[sourceName]++
		name := sourceName
		if seen[sourceName] > 1 {
			name = fmt.Sprintf("%s_%d", sourceName, seen[sourceName])
		}

		column := types.QueryColumn{
			Name:   name,
			DBType: lookupTypeName(typeMap, field.DataTypeOID),
		}
		if name != sourceName {
			column.SourceName = sourceName
		}

		columns = append(columns, column)
	}

	return columns
}

func buildQueryRow(columns []types.QueryColumn, fields []pgconn.FieldDescription, values []any) (map[string]any, error) {
	row := make(map[string]any, len(values))

	for index, column := range columns {
		normalized, err := normalizeQueryValue(values[index], fields[index].DataTypeOID)
		if err != nil {
			return nil, fmt.Errorf("normalize column %q: %w", column.Name, err)
		}

		row[column.Name] = normalized
	}

	return row, nil
}

func normalizeQueryValue(value any, oid uint32) (any, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return v, nil
	case string:
		if oid == pgtype.JSONOID || oid == pgtype.JSONBOID {
			return decodeJSONValue([]byte(v))
		}
		return v, nil
	case time.Time:
		return v.Format(time.RFC3339Nano), nil
	case json.RawMessage:
		return decodeJSONValue([]byte(v))
	case []byte:
		switch oid {
		case pgtype.ByteaOID:
			return base64.StdEncoding.EncodeToString(v), nil
		case pgtype.JSONOID, pgtype.JSONBOID:
			return decodeJSONValue(v)
		default:
			if utf8.Valid(v) {
				return string(v), nil
			}
			return base64.StdEncoding.EncodeToString(v), nil
		}
	}

	if valuer, ok := value.(driver.Valuer); ok {
		driverValue, err := valuer.Value()
		if err != nil {
			return nil, fmt.Errorf("driver value: %w", err)
		}
		return normalizeQueryValue(driverValue, oid)
	}

	reflectValue := reflect.ValueOf(value)
	switch reflectValue.Kind() {
	case reflect.Pointer, reflect.Interface:
		if reflectValue.IsNil() {
			return nil, nil
		}
		return normalizeQueryValue(reflectValue.Elem().Interface(), oid)
	case reflect.Slice, reflect.Array:
		out := make([]any, reflectValue.Len())
		for index := 0; index < reflectValue.Len(); index++ {
			item, err := normalizeQueryValue(reflectValue.Index(index).Interface(), 0)
			if err != nil {
				return nil, err
			}
			out[index] = item
		}
		return out, nil
	case reflect.Map:
		if reflectValue.Type().Key().Kind() != reflect.String {
			break
		}

		keys := reflectValue.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].String() < keys[j].String()
		})

		out := make(map[string]any, len(keys))
		for _, key := range keys {
			item, err := normalizeQueryValue(reflectValue.MapIndex(key).Interface(), 0)
			if err != nil {
				return nil, err
			}
			out[key.String()] = item
		}
		return out, nil
	}

	marshaled, err := json.Marshal(value)
	if err == nil {
		var decoded any
		if err := json.Unmarshal(marshaled, &decoded); err == nil {
			return decoded, nil
		}
	}

	return fmt.Sprintf("%v", value), nil
}

func decodeJSONValue(raw []byte) (any, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode JSON value: %w", err)
	}
	return decoded, nil
}

func lookupTypeName(typeMap *pgtype.Map, oid uint32) string {
	if dataType, ok := typeMap.TypeForOID(oid); ok {
		return dataType.Name
	}
	return fmt.Sprintf("oid:%d", oid)
}

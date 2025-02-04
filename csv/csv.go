package csv

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	pilosa "github.com/pilosa/go-pilosa"
)

// Format is the format of the data in the CSV file.
type Format uint

const (
	// RowIDColumnID formatted data is ROW_ID,COLUMN_ID.
	RowIDColumnID Format = iota
	// RowIDColumnKey formatted data is ROW_ID,COLUMN_KEY.
	RowIDColumnKey
	// RowKeyColumnID formatted data is ROW_KEY,COLUMN_ID.
	RowKeyColumnID
	// RowKeyColumnKey formatted data is ROW_KEY,COLUMN_ID.
	RowKeyColumnKey
	// ColumnID formatted data is COLUMN_ID. Valid only for value import.
	ColumnID
	// ColumnKey formatted data is COLUMN_KEY. Valud only for value import.
	ColumnKey
)

// ColumnUnmarshaller creates a RecordUnmarshaller for importing columns with the given format.
func ColumnUnmarshaller(format Format) RecordUnmarshaller {
	return ColumnUnmarshallerWithTimestamp(format, "")
}

// ColumnUnmarshallerWithTimestamp creates a RecordUnmarshaller for importing columns with the given format and timestamp format.
func ColumnUnmarshallerWithTimestamp(format Format, timestampFormat string) RecordUnmarshaller {
	return func(text string) (pilosa.Record, error) {
		var err error
		column := pilosa.Column{}
		parts := strings.Split(text, ",")
		if len(parts) < 2 {
			return nil, errors.New("Invalid CSV line")
		}

		hasRowKey := format == RowKeyColumnID || format == RowKeyColumnKey
		hasColumnKey := format == RowIDColumnKey || format == RowKeyColumnKey

		if hasRowKey {
			column.RowKey = parts[0]
		} else {
			column.RowID, err = strconv.ParseUint(parts[0], 10, 64)
			if err != nil {
				return nil, errors.New("Invalid row ID")
			}
		}

		if hasColumnKey {
			column.ColumnKey = parts[1]
		} else {
			column.ColumnID, err = strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				return nil, errors.New("Invalid column ID")
			}
		}

		timestamp := 0
		if len(parts) == 3 {
			if timestampFormat == "" {
				timestamp, err = strconv.Atoi(parts[2])
				if err != nil {
					return nil, err
				}
			} else {
				t, err := time.Parse(timestampFormat, parts[2])
				if err != nil {
					return nil, err
				}
				timestamp = int(t.Unix())
			}
		}
		column.Timestamp = int64(timestamp)

		return column, nil
	}
}

// RecordUnmarshaller is a function which creates a Record from a CSV file line with column data.
type RecordUnmarshaller func(text string) (pilosa.Record, error)

// Iterator reads records from a Reader.
// Each line should contain a single record in the following form:
// field1,field2,...
type Iterator struct {
	reader       io.Reader
	line         int
	scanner      *bufio.Scanner
	unmarshaller RecordUnmarshaller
}

// NewIterator creates a CSVIterator from a Reader.
func NewIterator(reader io.Reader, unmarshaller RecordUnmarshaller) *Iterator {
	return &Iterator{
		reader:       reader,
		line:         0,
		scanner:      bufio.NewScanner(reader),
		unmarshaller: unmarshaller,
	}
}

// NewColumnIterator creates a new iterator for column data.
func NewColumnIterator(format Format, reader io.Reader) *Iterator {
	return NewIterator(reader, ColumnUnmarshaller(format))
}

// NewColumnIteratorWithTimestampFormat creates a new iterator for column data with timestamp.
func NewColumnIteratorWithTimestampFormat(format Format, reader io.Reader, timestampFormat string) *Iterator {
	return NewIterator(reader, ColumnUnmarshallerWithTimestamp(format, timestampFormat))
}

// NewValueIterator creates a new iterator for value data.
func NewValueIterator(format Format, reader io.Reader) *Iterator {
	return NewIterator(reader, FieldValueUnmarshaller(format))
}

// NextRecord iterates on lines of a Reader.
// Returns io.EOF on end of iteration.
func (c *Iterator) NextRecord() (pilosa.Record, error) {
	if ok := c.scanner.Scan(); ok {
		c.line++
		text := strings.TrimSpace(c.scanner.Text())
		if text != "" {
			rc, err := c.unmarshaller(text)
			if err != nil {
				return nil, fmt.Errorf("%s at line: %d", err.Error(), c.line)
			}
			return rc, nil
		}
	}
	err := c.scanner.Err()
	if err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// FieldValueUnmarshaller is a function which creates a Record from a CSV file line with value data.
func FieldValueUnmarshaller(format Format) RecordUnmarshaller {
	return func(text string) (pilosa.Record, error) {
		parts := strings.Split(text, ",")
		if len(parts) < 2 {
			return nil, errors.New("Invalid CSV")
		}
		value, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, errors.New("Invalid value")
		}
		switch format {
		case ColumnID:
			columnID, err := strconv.ParseUint(parts[0], 10, 64)
			if err != nil {
				return nil, errors.New("Invalid column ID at line: %d")
			}
			return pilosa.FieldValue{
				ColumnID: uint64(columnID),
				Value:    value,
			}, nil
		case ColumnKey:
			return pilosa.FieldValue{
				ColumnKey: parts[0],
				Value:     value,
			}, nil
		default:
			return nil, fmt.Errorf("Invalid format: %d", format)
		}
	}
}

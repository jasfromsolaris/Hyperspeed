package datasetread

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/parquet-go/parquet-go"
)

const (
	MaxProcessBytes = 256 << 20 // cap downloads / local reads for infer + query
	MaxPreviewRows  = 500
	MaxQueryRows    = 1000
)

type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type Schema struct {
	Columns []ColumnInfo `json:"columns"`
}

// FilterOp is one of: eq, ne, gt, lt, contains.
type Filter struct {
	Column string `json:"column"`
	Op     string `json:"op"`
	Value  string `json:"value"`
}

type QueryRequest struct {
	Columns []string  `json:"columns"` // empty = all
	Filters []Filter  `json:"filters"`
	Limit   int       `json:"limit"`
	Offset  int       `json:"offset"`
}

type TableResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

func clampLimit(n, max int) int {
	if n <= 0 {
		return max
	}
	if n > max {
		return max
	}
	return n
}

func parquetColumnNames(pf *parquet.File) []string {
	var names []string
	for _, path := range pf.Schema().Columns() {
		names = append(names, strings.Join(path, "."))
	}
	return names
}

func rowToStrings(row parquet.Row, numCols int) []string {
	out := make([]string, numCols)
	i := 0
	row.Range(func(_ int, values []parquet.Value) bool {
		if i >= numCols {
			return false
		}
		if len(values) == 0 {
			out[i] = ""
		} else {
			out[i] = values[0].String()
		}
		i++
		return true
	})
	return out
}

func matchFilters(cols []string, row []string, filters []Filter) bool {
	idx := make(map[string]int, len(cols))
	for i, c := range cols {
		idx[c] = i
	}
	for _, f := range filters {
		j, ok := idx[f.Column]
		if !ok {
			return false
		}
		cell := row[j]
		switch f.Op {
		case "eq":
			if cell != f.Value {
				return false
			}
		case "ne":
			if cell == f.Value {
				return false
			}
		case "contains":
			if !strings.Contains(cell, f.Value) {
				return false
			}
		case "gt", "lt":
			a, e1 := strconv.ParseFloat(cell, 64)
			b, e2 := strconv.ParseFloat(f.Value, 64)
			if e1 != nil || e2 != nil {
				if f.Op == "gt" {
					if strings.Compare(cell, f.Value) <= 0 {
						return false
					}
				} else {
					if strings.Compare(cell, f.Value) >= 0 {
						return false
					}
				}
				continue
			}
			if f.Op == "gt" && !(a > b) {
				return false
			}
			if f.Op == "lt" && !(a < b) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func outputColumnOrder(base []string, pick []string) []string {
	if len(pick) == 0 {
		return append([]string(nil), base...)
	}
	valid := make(map[string]bool, len(base))
	for _, b := range base {
		valid[b] = true
	}
	var out []string
	seen := make(map[string]bool)
	for _, p := range pick {
		if valid[p] && !seen[p] {
			out = append(out, p)
			seen[p] = true
		}
	}
	if len(out) == 0 {
		return append([]string(nil), base...)
	}
	return out
}

func projectRowValues(baseCols []string, row []string, outCols []string) []string {
	idx := make(map[string]int, len(baseCols))
	for i, c := range baseCols {
		idx[c] = i
	}
	pr := make([]string, len(outCols))
	for i, name := range outCols {
		if j, ok := idx[name]; ok && j < len(row) {
			pr[i] = row[j]
		}
	}
	return pr
}

// InferParquet reads footer/schema only via OpenFile.
func InferParquet(path string) (Schema, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return Schema{}, 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Schema{}, 0, err
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		return Schema{}, 0, err
	}
	names := parquetColumnNames(pf)
	var cols []ColumnInfo
	for _, name := range names {
		cols = append(cols, ColumnInfo{Name: name, Type: "string"})
	}
	schema := Schema{Columns: cols}
	return schema, pf.NumRows(), nil
}

func SchemaJSON(s Schema) (json.RawMessage, error) {
	b, err := json.Marshal(s)
	return json.RawMessage(b), err
}

// PreviewParquet returns up to limit rows (all columns).
func PreviewParquet(path string, limit int) (TableResult, error) {
	limit = clampLimit(limit, MaxPreviewRows)
	f, err := os.Open(path)
	if err != nil {
		return TableResult{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return TableResult{}, err
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		return TableResult{}, err
	}
	cols := parquetColumnNames(pf)
	r := parquet.NewGenericReader[parquet.Row](f)
	defer r.Close()
	buf := make([]parquet.Row, min(256, limit))
	var rows [][]string
	for len(rows) < limit {
		n, err := r.Read(buf)
		for i := 0; i < n && len(rows) < limit; i++ {
			rows = append(rows, rowToStrings(buf[i].Clone(), len(cols)))
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return TableResult{}, err
		}
		if n == 0 {
			break
		}
	}
	return TableResult{Columns: cols, Rows: rows}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// QueryParquet scans rows with optional filters (in-memory, row-group sequential).
func QueryParquet(path string, req QueryRequest) (TableResult, error) {
	limit := clampLimit(req.Limit, MaxQueryRows)
	if req.Offset < 0 {
		req.Offset = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return TableResult{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return TableResult{}, err
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		return TableResult{}, err
	}
	baseCols := parquetColumnNames(pf)
	outCols := outputColumnOrder(baseCols, req.Columns)
	r := parquet.NewGenericReader[parquet.Row](f)
	defer r.Close()
	buf := make([]parquet.Row, 256)
	var out [][]string
	skipped := 0
	for {
		n, err := r.Read(buf)
		for i := 0; i < n; i++ {
			row := rowToStrings(buf[i].Clone(), len(baseCols))
			if !matchFilters(baseCols, row, req.Filters) {
				continue
			}
			if skipped < req.Offset {
				skipped++
				continue
			}
			out = append(out, projectRowValues(baseCols, row, outCols))
			if len(out) >= limit {
				return TableResult{Columns: outCols, Rows: out}, nil
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return TableResult{}, err
		}
		if n == 0 {
			break
		}
	}
	return TableResult{Columns: outCols, Rows: out}, nil
}

// InferCSV reads up to maxBytes from path for header + type guess (types are string for V1).
func InferCSV(path string, maxBytes int64) (Schema, int64, error) {
	if maxBytes <= 0 || maxBytes > MaxProcessBytes {
		maxBytes = MaxProcessBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return Schema{}, 0, err
	}
	defer f.Close()
	sniff := make([]byte, min64(maxBytes, 1<<20))
	n, _ := f.Read(sniff)
	r := csv.NewReader(bytes.NewReader(sniff[:n]))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return Schema{}, 0, fmt.Errorf("csv header: %w", err)
	}
	var cols []ColumnInfo
	for _, h := range header {
		cols = append(cols, ColumnInfo{Name: strings.TrimSpace(h), Type: "string"})
	}
	// Rough row count: count newlines in file (cheap upper bound).
	st, err := f.Stat()
	if err != nil {
		return Schema{Columns: cols}, 0, nil
	}
	size := st.Size()
	// recount lines in full file for estimate (capped read)
	estimate, _ := estimateCSVRows(path, min64(size, maxBytes))
	return Schema{Columns: cols}, estimate, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func estimateCSVRows(path string, maxRead int64) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var lines int64
	buf := make([]byte, 64*1024)
	var total int64
	for total < maxRead {
		n, err := f.Read(buf)
		if n == 0 {
			break
		}
		total += int64(n)
		for _, b := range buf[:n] {
			if b == '\n' {
				lines++
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return lines, err
		}
	}
	if lines > 0 {
		return lines - 1, nil // minus header
	}
	return 0, nil
}

// PreviewCSV reads up to maxBytes from file start.
func PreviewCSV(path string, limit int, maxBytes int64) (TableResult, error) {
	limit = clampLimit(limit, MaxPreviewRows)
	if maxBytes <= 0 || maxBytes > MaxProcessBytes {
		maxBytes = MaxProcessBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return TableResult{}, err
	}
	defer f.Close()
	chunk := make([]byte, maxBytes)
	n, err := io.ReadFull(f, chunk)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return TableResult{}, err
	}
	chunk = chunk[:n]
	r := csv.NewReader(bytes.NewReader(chunk))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return TableResult{}, err
	}
	if len(records) == 0 {
		return TableResult{}, errors.New("empty csv")
	}
	header := records[0]
	var rows [][]string
	for i := 1; i < len(records) && len(rows) < limit; i++ {
		rows = append(rows, records[i])
	}
	return TableResult{Columns: header, Rows: rows}, nil
}

// QueryCSV reads sequential rows from start of file up to maxBytes window (MVP).
func QueryCSV(path string, req QueryRequest, maxBytes int64) (TableResult, error) {
	limit := clampLimit(req.Limit, MaxQueryRows)
	if maxBytes <= 0 || maxBytes > MaxProcessBytes {
		maxBytes = MaxProcessBytes
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return TableResult{}, err
	}
	defer f.Close()
	chunk := make([]byte, maxBytes)
	n, err := io.ReadFull(f, chunk)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return TableResult{}, err
	}
	chunk = chunk[:n]
	r := csv.NewReader(bytes.NewReader(chunk))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return TableResult{}, err
	}
	if len(records) == 0 {
		return TableResult{}, errors.New("empty csv")
	}
	baseCols := make([]string, len(records[0]))
	copy(baseCols, records[0])
	outCols := outputColumnOrder(baseCols, req.Columns)
	var out [][]string
	skipped := 0
	for i := 1; i < len(records); i++ {
		row := records[i]
		for len(row) < len(baseCols) {
			row = append(row, "")
		}
		if len(row) > len(baseCols) {
			row = row[:len(baseCols)]
		}
		if !matchFilters(baseCols, row, req.Filters) {
			continue
		}
		if skipped < req.Offset {
			skipped++
			continue
		}
		out = append(out, projectRowValues(baseCols, row, outCols))
		if len(out) >= limit {
			return TableResult{Columns: outCols, Rows: out}, nil
		}
	}
	return TableResult{Columns: outCols, Rows: out}, nil
}

package idmap

import (
	"context"
)

// Shared test doubles for the ID-map package. The behaviour tests live in
// service_test.go and repository_test.go, matching the constitution's
// companion-file convention.

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (f fakeRow) Scan(dest ...any) error {
	return f.scanFn(dest...)
}

type fakeRepo struct {
	id         uint64
	found      bool
	err        error
	lastString string
	lastNS     string
	lastType   string
}

func (f *fakeRepo) GetOrCreate(_ context.Context, stringID, namespace, entityType string) (uint64, error) {
	f.lastString = stringID
	f.lastNS = namespace
	f.lastType = entityType
	return f.id, f.err
}

func (f *fakeRepo) Lookup(_ context.Context, stringID, namespace, entityType string) (numericID uint64, found bool, err error) {
	f.lastString = stringID
	f.lastNS = namespace
	f.lastType = entityType
	return f.id, f.found, f.err
}

func (f *fakeRepo) LookupBatch(_ context.Context, stringIDs []string, namespace, entityType string) (map[string]uint64, error) {
	f.lastNS = namespace
	f.lastType = entityType
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]uint64, len(stringIDs))
	for i, id := range stringIDs {
		out[id] = f.id + uint64(i)
	}
	return out, nil
}

func (f *fakeRepo) GetOrCreateBatch(_ context.Context, stringIDs []string, namespace, entityType string) (map[string]uint64, error) {
	f.lastNS = namespace
	f.lastType = entityType
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]uint64, len(stringIDs))
	for i, id := range stringIDs {
		out[id] = f.id + uint64(i)
	}
	return out, nil
}

type fakeRows struct {
	rows    [][]any // each row: [string_id(string), numeric_id(int64)]
	idx     int
	scanErr error
	rowsErr error
	closed  bool
}

func (f *fakeRows) Next() bool { return f.idx < len(f.rows) }

func (f *fakeRows) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	row := f.rows[f.idx]
	f.idx++
	*dest[0].(*string) = row[0].(string)
	*dest[1].(*int64) = row[1].(int64)
	return nil
}

func (f *fakeRows) Err() error { return f.rowsErr }

func (f *fakeRows) Close() { f.closed = true }

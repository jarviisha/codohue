package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jarviisha/codohue/internal/objects"
)

// fakeObjectsRepo satisfies the objects repository surface so the adapter can
// be driven without PostgreSQL. It records which path reached it.
type fakeObjectsRepo struct {
	upserts []string // "ns/object/author"
	err     error
}

func (f *fakeObjectsRepo) Upsert(_ context.Context, ns, objectID, author string) (*objects.Object, error) {
	f.upserts = append(f.upserts, ns+"/"+objectID+"/"+author)
	if f.err != nil {
		return nil, f.err
	}
	return &objects.Object{Namespace: ns, ObjectID: objectID, AuthorSubjectID: author, UpdatedAt: time.Now()}, nil
}

func (f *fakeObjectsRepo) Get(context.Context, string, string) (*objects.Object, error) {
	return nil, nil
}

func (f *fakeObjectsRepo) Delete(context.Context, string, string) error { return nil }

// stubTx is a pgx.Tx that records the statements routed through it. Only
// QueryRow is meaningful: it is the single call the objects upsert makes, and
// observing it is what proves the write joined THIS transaction rather than
// taking the service's own pool connection.
type stubTx struct {
	queries []string
}

func (s *stubTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	s.queries = append(s.queries, sql)
	return stubRow{}
}

func (s *stubTx) Begin(context.Context) (pgx.Tx, error) { return nil, errors.New("not used") }
func (s *stubTx) Commit(context.Context) error          { return nil }
func (s *stubTx) Rollback(context.Context) error        { return nil }
func (s *stubTx) LargeObjects() pgx.LargeObjects        { return pgx.LargeObjects{} }
func (s *stubTx) Conn() *pgx.Conn                       { return nil }
func (s *stubTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}
func (s *stubTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("not used")
}
func (s *stubTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, errors.New("not used")
}
func (s *stubTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("not used")
}
func (s *stubTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("not used")
}

// stubRow satisfies the five-column scan the objects upsert performs.
type stubRow struct{}

func (stubRow) Scan(dest ...any) error {
	values := []any{"ns", "o1", "u1", time.Now(), time.Now()}
	for i, d := range dest {
		if i >= len(values) {
			break
		}
		switch target := d.(type) {
		case *string:
			*target, _ = values[i].(string)
		case *time.Time:
			*target, _ = values[i].(time.Time)
		}
	}
	return nil
}

// Catalog ingest passes its open transaction so the attribution commits with
// the content. If the adapter ignored it and used the service's own pool
// connection instead, a rolled-back ingest would still leave the author behind
// — the exact split-success the atomic write exists to prevent.
func TestObjectAttributionAdapter_TransactionCarriesTheWrite(t *testing.T) {
	repo := &fakeObjectsRepo{}
	adapter := &objectAttributionAdapter{svc: objects.NewService(repo)}
	tx := &stubTx{}

	if err := adapter.SetAuthorTx(context.Background(), tx, "ns", "o1", "u1"); err != nil {
		t.Fatalf("SetAuthorTx: %v", err)
	}

	if len(tx.queries) != 1 {
		t.Fatalf("transaction received %d statement(s), want the attribution upsert", len(tx.queries))
	}
	if len(repo.upserts) != 0 {
		t.Errorf("the write bypassed the transaction and used the service repository: %v", repo.upserts)
	}
}

// A nil transaction is the pool-less path unit tests drive. It must still
// perform the write — through the service's own repository — rather than
// silently doing nothing.
func TestObjectAttributionAdapter_NilTransactionFallsBackToTheService(t *testing.T) {
	repo := &fakeObjectsRepo{}
	adapter := &objectAttributionAdapter{svc: objects.NewService(repo)}

	if err := adapter.SetAuthorTx(context.Background(), nil, "ns", "o1", "u1"); err != nil {
		t.Fatalf("SetAuthorTx: %v", err)
	}

	if len(repo.upserts) != 1 || repo.upserts[0] != "ns/o1/u1" {
		t.Fatalf("service repository received %v, want one ns/o1/u1 write", repo.upserts)
	}
}

// Absence means "unspecified", not "clear it": a catalog re-ingest that omits
// the author must not wipe an attribution set through the objects endpoint.
// The rule lives in the objects service, so the adapter must route through it
// rather than writing directly.
func TestObjectAttributionAdapter_EmptyAuthorIsANoOpOnBothPaths(t *testing.T) {
	for _, tc := range []struct {
		name string
		tx   pgx.Tx
	}{
		{"with a transaction", &stubTx{}},
		{"without a transaction", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeObjectsRepo{}
			adapter := &objectAttributionAdapter{svc: objects.NewService(repo)}

			for _, author := range []string{"", "   ", "\t"} {
				if err := adapter.SetAuthorTx(context.Background(), tc.tx, "ns", "o1", author); err != nil {
					t.Fatalf("author %q: %v", author, err)
				}
			}

			if len(repo.upserts) != 0 {
				t.Errorf("an absent author reached the repository: %v", repo.upserts)
			}
			if stub, ok := tc.tx.(*stubTx); ok && len(stub.queries) != 0 {
				t.Errorf("an absent author reached the transaction: %v", stub.queries)
			}
		})
	}
}

// A failing attribution must surface so the catalog transaction rolls back;
// swallowing it is what produced 202-with-no-author before.
func TestObjectAttributionAdapter_PropagatesWriteFailures(t *testing.T) {
	repo := &fakeObjectsRepo{err: errors.New("objects table down")}
	adapter := &objectAttributionAdapter{svc: objects.NewService(repo)}

	if err := adapter.SetAuthorTx(context.Background(), nil, "ns", "o1", "u1"); err == nil {
		t.Fatal("a failed attribution must be reported to the caller")
	}
}

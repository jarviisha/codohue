package nslifecycle

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LockMode selects shared writer or exclusive lifecycle coordination.
type LockMode string

// Writers take LockShared so they run concurrently with each other; delete,
// recreate and reset take LockExclusive to serialize against all of them.
const (
	LockShared    LockMode = "shared"
	LockExclusive LockMode = "exclusive"
)

// Lock is a held PostgreSQL session advisory lock.
type Lock interface{ Release(context.Context) error }

// Locker acquires named locks. Service always acquires global before namespace.
type Locker interface {
	Acquire(context.Context, string, LockMode) (Lock, error)
}

type heldKey struct {
	key  int64
	mode LockMode
}

type postgresLock struct {
	conn *pgxpool.Conn
	held []heldKey
	once sync.Once
	err  error
}

func unlockQuery(mode LockMode) string {
	if mode == LockShared {
		return `SELECT pg_advisory_unlock_shared($1)`
	}
	return `SELECT pg_advisory_unlock($1)`
}

// Release drops every advisory lock on the session and returns the pooled
// connection. It is idempotent: releasing twice must not return the connection
// twice. Keys unlock in reverse acquisition order, and one failure does not
// skip the rest — a leaked advisory lock outlives the request.
func (l *postgresLock) Release(ctx context.Context) error {
	l.once.Do(func() {
		var errs []error
		for i := len(l.held) - 1; i >= 0; i-- {
			if _, err := l.conn.Exec(ctx, unlockQuery(l.held[i].mode), l.held[i].key); err != nil {
				errs = append(errs, err)
			}
		}
		l.err = errors.Join(errs...)
		l.conn.Release()
	})
	return l.err
}

// PostgresLocker holds session advisory locks on its own dedicated pool.
//
// The pool is deliberately separate from the caller's work pool. A fenced write
// holds its lock session for the whole request *and* needs a second connection
// to do the actual write; drawing both from one pool deadlocks as soon as
// concurrent writes reach the pool size, because every request then holds a
// connection while waiting for one only another blocked request could release.
// Two pools make that cycle impossible — a lock holder can always obtain a work
// connection and finish. See TestPostgresLockerDoesNotDeadlockASaturatedPool.
type PostgresLocker struct {
	pool  *pgxpool.Pool
	owned bool
}

// NewPostgresLocker builds a locker backed by its own pool, derived from the
// work pool's configuration so it needs no separate DSN. Sized to the work
// pool: beyond that many concurrent fenced writes, callers queue for a lock
// session rather than deadlocking.
func NewPostgresLocker(db *pgxpool.Pool) (*PostgresLocker, error) {
	cfg := db.Config().Copy()
	if cfg.MaxConns < 4 {
		cfg.MaxConns = 4
	}
	// Lock sessions are idle most of their life; keeping a warm minimum would
	// hold connections a busy work pool may need.
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("create lifecycle lock pool: %w", err)
	}
	return &PostgresLocker{pool: pool, owned: true}, nil
}

// Close releases the dedicated lock pool.
func (l *PostgresLocker) Close() {
	if l != nil && l.owned && l.pool != nil {
		l.pool.Close()
	}
}

func lockKey(name string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte("codohue:nslifecycle:" + name))
	return int64(hash.Sum64())
}

func lockQuery(mode LockMode) string {
	if mode == LockShared {
		return `SELECT pg_advisory_lock_shared($1)`
	}
	return `SELECT pg_advisory_lock($1)`
}

// Acquire blocks until the named lock is held in the requested mode.
func (l *PostgresLocker) Acquire(ctx context.Context, name string, mode LockMode) (Lock, error) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lifecycle lock connection: %w", err)
	}
	if _, err := conn.Exec(ctx, lockQuery(mode), lockKey(name)); err != nil {
		conn.Release()
		return nil, fmt.Errorf("acquire lifecycle lock %q: %w", name, err)
	}
	return &postgresLock{conn: conn, held: []heldKey{{key: lockKey(name), mode: mode}}}, nil
}

// AcquirePair takes the global lock and then the namespace lock on a *single*
// session, and is why Service.acquirePair is not two Acquire calls.
//
// Advisory locks are per-session, so two locks on two pooled connections means
// a request holds one connection while blocking for a second. N concurrent
// writers against a pool of N then wedge permanently: each holds the global
// lock and waits forever for a connection to take the namespace lock. One
// session for both removes that cycle; the fixed global-before-namespace order
// is preserved.
func (l *PostgresLocker) AcquirePair(ctx context.Context, namespace string, namespaceMode LockMode) (Lock, error) {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lifecycle lock connection: %w", err)
	}
	held := &postgresLock{conn: conn}
	for _, want := range []heldKey{
		{key: lockKey("global"), mode: LockShared},
		{key: lockKey("namespace:" + namespace), mode: namespaceMode},
	} {
		if _, err := conn.Exec(ctx, lockQuery(want.mode), want.key); err != nil {
			// Release what this session already holds before handing the
			// connection back, or the next borrower inherits the locks.
			if releaseErr := held.Release(context.WithoutCancel(ctx)); releaseErr != nil {
				slog.Warn("release partial lifecycle lock pair failed", "namespace", namespace, "error", releaseErr)
			}
			return nil, fmt.Errorf("acquire lifecycle lock pair for %q: %w", namespace, err)
		}
		held.held = append(held.held, want)
	}
	return held, nil
}

type leaseContextKey struct{}

type leaseValue struct {
	Namespace  string
	Generation int64
	Mode       LockMode
}

// RequireNamespaceLease rejects mapping or mutation code that is reached
// without a current generation lease for namespace. Callers that also need the
// generation read it with LeaseGeneration rather than passing an expected one:
// the lease is the authority on which generation is current, so a caller-
// supplied value could only ever disagree with it.
func RequireNamespaceLease(ctx context.Context, namespace string) error {
	lease, ok := ctx.Value(leaseContextKey{}).(leaseValue)
	if !ok || lease.Namespace != namespace || lease.Generation < 1 {
		return ErrLeaseRequired
	}
	return nil
}

// LeaseGeneration returns the generation carried by the namespace lifecycle
// lease. Read-only callers without a lease should resolve generation from
// namespace configuration instead.
func LeaseGeneration(ctx context.Context, namespace string) (int64, bool) {
	lease, ok := ctx.Value(leaseContextKey{}).(leaseValue)
	if !ok || lease.Namespace != namespace || lease.Generation < 1 {
		return 0, false
	}
	return lease.Generation, true
}

// ContextWithLease carries a lease already held by a transaction/composition
// adapter. Most callers should use Service.WithWriter instead.
func ContextWithLease(ctx context.Context, namespace string, generation int64, mode LockMode) context.Context {
	return context.WithValue(ctx, leaseContextKey{}, leaseValue{Namespace: namespace, Generation: generation, Mode: mode})
}

// Service coordinates durable state with fixed-order advisory locks.
type Service struct {
	store  Store
	locker Locker
	now    func() time.Time
}

// NewService wires durable lifecycle state to the lock provider.
func NewService(store Store, locker Locker) *Service {
	return &Service{store: store, locker: locker, now: time.Now}
}

// pairLocker takes both lifecycle locks on one session. PostgresLocker
// implements it; the in-memory fakes in tests do not, and fall back to the
// two-lock path below, which still exercises the ordering contract.
type pairLocker interface {
	AcquirePair(ctx context.Context, namespace string, namespaceMode LockMode) (Lock, error)
}

// acquirePair takes the global lock before the namespace lock, always in that
// order. Any caller that reversed it could deadlock against one that did not.
//
// Against a real database both locks land on one session, so a request never
// holds a pooled connection while waiting for another one.
func (s *Service) acquirePair(ctx context.Context, namespace string, namespaceMode LockMode) (global, namespaceLock Lock, err error) {
	if pairwise, ok := s.locker.(pairLocker); ok {
		both, pairErr := pairwise.AcquirePair(ctx, namespace, namespaceMode)
		if pairErr != nil {
			return nil, nil, pairErr
		}
		// releasePair releases namespace-then-global; the single held lock
		// unlocks both keys, so it stands in for the namespace half and the
		// global half is a no-op.
		return noopLock{}, both, nil
	}
	global, err = s.locker.Acquire(ctx, "global", LockShared)
	if err != nil {
		return nil, nil, err
	}
	namespaceLock, err = s.locker.Acquire(ctx, "namespace:"+namespace, namespaceMode)
	if err != nil {
		if releaseErr := global.Release(context.WithoutCancel(ctx)); releaseErr != nil {
			slog.Warn("release global lifecycle lock failed", "namespace", namespace, "error", releaseErr)
		}
		return nil, nil, err
	}
	return global, namespaceLock, nil
}

// noopLock stands in for the global half when both locks share one session.
type noopLock struct{}

// Release does nothing: the paired lock released both keys already.
func (noopLock) Release(context.Context) error { return nil }

func releasePair(ctx context.Context, namespaceLock, global Lock) error {
	releaseCtx := context.WithoutCancel(ctx)
	return errors.Join(namespaceLock.Release(releaseCtx), global.Release(releaseCtx))
}

// WithWriter acquires global-shared then namespace-shared, rereads durable
// gates, and carries the matching lease in context through the mutation.
func (s *Service) WithWriter(ctx context.Context, namespace string, fn func(context.Context, *NamespaceLifecycle) error) error {
	if existing, ok := ctx.Value(leaseContextKey{}).(leaseValue); ok && existing.Namespace == namespace {
		return fn(ctx, &NamespaceLifecycle{Namespace: namespace, Generation: existing.Generation, State: StateActive})
	}
	global, namespaceLock, err := s.acquirePair(ctx, namespace, LockShared)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := releasePair(ctx, namespaceLock, global); releaseErr != nil {
			slog.Warn("release lifecycle locks failed", "namespace", namespace, "error", releaseErr)
		}
	}()
	system, err := s.store.GetSystem(ctx)
	if err != nil {
		return fmt.Errorf("read system lifecycle after lock: %w", err)
	}
	if system.State != SystemActive {
		return ErrSystemResetting
	}
	lifecycle, err := s.store.GetNamespace(ctx, namespace)
	if err != nil {
		return err
	}
	if lifecycle.State != StateActive {
		return ErrNamespaceNotActive
	}
	leased := context.WithValue(ctx, leaseContextKey{}, leaseValue{Namespace: namespace, Generation: lifecycle.Generation, Mode: LockShared})
	return fn(leased, lifecycle)
}

// WithNamespaceExclusive serializes delete/recreate with all namespace writers.
func (s *Service) WithNamespaceExclusive(ctx context.Context, namespace string, fn func(context.Context, *NamespaceLifecycle) error) (err error) {
	global, namespaceLock, err := s.acquirePair(ctx, namespace, LockExclusive)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, releasePair(ctx, namespaceLock, global)) }()
	system, err := s.store.GetSystem(ctx)
	if err != nil {
		return err
	}
	if system.State != SystemActive {
		return ErrSystemResetting
	}
	lifecycle, err := s.store.GetNamespace(ctx, namespace)
	if err != nil {
		return err
	}
	leased := context.WithValue(ctx, leaseContextKey{}, leaseValue{Namespace: namespace, Generation: lifecycle.Generation, Mode: LockExclusive})
	return fn(leased, lifecycle)
}

// Activate creates or recreates a lifecycle under the exclusive namespace lease.
func (s *Service) Activate(ctx context.Context, namespace string) (out *NamespaceLifecycle, err error) {
	global, namespaceLock, err := s.acquirePair(ctx, namespace, LockExclusive)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, releasePair(ctx, namespaceLock, global)) }()
	system, err := s.store.GetSystem(ctx)
	if err != nil {
		return nil, err
	}
	if system.State != SystemActive {
		return nil, ErrSystemResetting
	}
	return s.store.Activate(ctx, namespace)
}

// WithGlobalExclusive blocks all new writers while reset/repair operations run.
func (s *Service) WithGlobalExclusive(ctx context.Context, fn func(context.Context, *SystemLifecycle) error) (err error) {
	lock, err := s.locker.Acquire(ctx, "global", LockExclusive)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lock.Release(context.WithoutCancel(ctx))) }()
	system, err := s.store.GetSystem(ctx)
	if err != nil {
		return err
	}
	return fn(ctx, system)
}

// DisableLegacyEnvelopes permanently closes generation-less work after adoption evidence.
func (s *Service) DisableLegacyEnvelopes(ctx context.Context, adoptionEvidence string) (bool, error) {
	if adoptionEvidence == "" {
		return false, ErrAdoptionEvidence
	}
	var changed bool
	err := s.WithGlobalExclusive(ctx, func(ctx context.Context, _ *SystemLifecycle) error {
		var err error
		changed, err = s.store.DisableLegacy(ctx, adoptionEvidence, s.now().UTC())
		return err
	})
	return changed, err
}

// EvaluateEnvelope applies the generation-1 compatibility rule without
// treating durable-store errors as stale work.
func (s *Service) EvaluateEnvelope(ctx context.Context, namespace string, generation *int64) (EnvelopeDisposition, error) {
	lifecycle, err := s.store.GetNamespace(ctx, namespace)
	if err != nil {
		return 0, err
	}
	if lifecycle.State != StateActive {
		return EnvelopeStale, nil
	}
	if generation == nil || *generation == 0 {
		if lifecycle.Generation == 1 && lifecycle.LegacyMessagesAllowed {
			return EnvelopeProcess, nil
		}
		return EnvelopeStale, nil
	}
	if *generation != lifecycle.Generation {
		return EnvelopeStale, nil
	}
	return EnvelopeProcess, nil
}

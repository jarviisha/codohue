package nslifecycle

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LockMode selects shared writer or exclusive lifecycle coordination.
type LockMode string

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

type postgresLock struct {
	conn *pgxpool.Conn
	key  int64
	mode LockMode
	once sync.Once
	err  error
}

func (l *postgresLock) Release(ctx context.Context) error {
	l.once.Do(func() {
		query := `SELECT pg_advisory_unlock($1)`
		if l.mode == LockShared {
			query = `SELECT pg_advisory_unlock_shared($1)`
		}
		_, l.err = l.conn.Exec(ctx, query, l.key)
		l.conn.Release()
	})
	return l.err
}

// PostgresLocker holds session locks on dedicated pool connections.
type PostgresLocker struct{ db *pgxpool.Pool }

func NewPostgresLocker(db *pgxpool.Pool) *PostgresLocker { return &PostgresLocker{db: db} }

func lockKey(name string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte("codohue:nslifecycle:" + name))
	return int64(hash.Sum64())
}

func (l *PostgresLocker) Acquire(ctx context.Context, name string, mode LockMode) (Lock, error) {
	conn, err := l.db.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire lifecycle lock connection: %w", err)
	}
	query := `SELECT pg_advisory_lock($1)`
	if mode == LockShared {
		query = `SELECT pg_advisory_lock_shared($1)`
	}
	if _, err := conn.Exec(ctx, query, lockKey(name)); err != nil {
		conn.Release()
		return nil, fmt.Errorf("acquire lifecycle lock %q: %w", name, err)
	}
	return &postgresLock{conn: conn, key: lockKey(name), mode: mode}, nil
}

type leaseContextKey struct{}

type leaseValue struct {
	Namespace  string
	Generation int64
	Mode       LockMode
}

// RequireLease rejects mapping or mutation code that is reached without the
// matching lifecycle generation lease.
func RequireLease(ctx context.Context, namespace string, generation int64) error {
	lease, ok := ctx.Value(leaseContextKey{}).(leaseValue)
	if !ok || lease.Namespace != namespace || lease.Generation != generation {
		return ErrLeaseRequired
	}
	return nil
}

// RequireNamespaceLease checks that the context carries any current
// generation lease for namespace. Use RequireLease when generation is known.
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

func NewService(store Store, locker Locker) *Service {
	return &Service{store: store, locker: locker, now: time.Now}
}

func (s *Service) acquirePair(ctx context.Context, namespace string, namespaceMode LockMode) (Lock, Lock, error) {
	global, err := s.locker.Acquire(ctx, "global", LockShared)
	if err != nil {
		return nil, nil, err
	}
	namespaceLock, err := s.locker.Acquire(ctx, "namespace:"+namespace, namespaceMode)
	if err != nil {
		_ = global.Release(context.WithoutCancel(ctx))
		return nil, nil, err
	}
	return global, namespaceLock, nil
}

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
	defer func() { _ = releasePair(ctx, namespaceLock, global) }()
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

package db

import (
	"sync"

	"code.cloudfoundry.org/lager"
	"github.com/jackc/pgx"
)

//go:generate counterfeiter . LockFactory

type LockFactory interface {
	NewLock(logger lager.Logger, lockID int) Lock
}

type lockFactory struct {
	conn  *pgx.Conn
	locks lockRepo
}

func NewLockFactory(conn *pgx.Conn) LockFactory {
	return &lockFactory{
		conn:  conn,
		locks: lockRepo{},
	}
}

func (f *lockFactory) NewLock(logger lager.Logger, lockID int) Lock {
	return &lock{
		conn:   f.conn,
		logger: logger,
		lockID: lockID,
		locks:  f.locks,
		mutex:  &sync.Mutex{},
	}
}

//go:generate counterfeiter . Lock

type Lock interface {
	Acquire() (bool, error)
	Release() error
	AfterRelease(func() error)
}

type lock struct {
	conn   *pgx.Conn
	logger lager.Logger

	mutex *sync.Mutex // to protect db connection access and locks repo

	lockID int
	locks  lockRepo

	afterRelease func() error
}

func (l *lock) Acquire() (bool, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.locks.IsRegistered(l.lockID) {
		return false, nil
	}

	var signed bool
	err := l.conn.QueryRow(`SELECT pg_try_advisory_lock($1)`, l.lockID).Scan(&signed)
	if err != nil {
		return false, err
	}

	l.locks.Register(l.lockID)

	return signed, nil
}

func (l *lock) Release() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	_, err := l.conn.Exec(`SELECT pg_advisory_unlock($1)`, l.lockID)
	if err != nil {
		return err
	}

	l.locks.Unregister(l.lockID)

	if l.afterRelease != nil {
		return l.afterRelease()
	}

	return nil
}

func (l *lock) AfterRelease(afterReleaseFunc func() error) {
	l.afterRelease = afterReleaseFunc
}

type lockRepo map[int]bool

func (lr lockRepo) IsRegistered(lockID int) bool {
	if _, ok := lr[lockID]; ok {
		return true
	}
	return false
}

func (lr lockRepo) Register(lockID int) {
	lr[lockID] = true
}

func (lr lockRepo) Unregister(lockID int) {
	delete(lr, lockID)
}

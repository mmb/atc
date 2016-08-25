package db

import (
	"sync"

	"code.cloudfoundry.org/lager"
	"github.com/jackc/pgx"
)

//go:generate counterfeiter . LeaseFactory

type LeaseFactory interface {
	NewLease(logger lager.Logger, lockID int) Lease
}

type leaseFactory struct {
	conn  *pgx.Conn
	locks lockRepo
}

func NewLeaseFactory(conn *pgx.Conn) LeaseFactory {
	return &leaseFactory{
		conn:  conn,
		locks: lockRepo{},
	}
}

func (f *leaseFactory) NewLease(logger lager.Logger, lockID int) Lease {
	return &lease{
		conn:   f.conn,
		logger: logger,
		lockID: lockID,
		locks:  f.locks,
		mutex:  &sync.Mutex{},
	}
}

//go:generate counterfeiter . Lease

type Lease interface {
	AttemptSign() (bool, error)
	Break() error
	AfterBreak(func() error)
}

type lease struct {
	conn   *pgx.Conn
	logger lager.Logger

	mutex *sync.Mutex // to protect db connection access and locks repo

	lockID int
	locks  lockRepo

	afterBreak func() error
}

func (l *lease) AttemptSign() (bool, error) {
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

func (l *lease) Break() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	_, err := l.conn.Exec(`SELECT pg_advisory_unlock($1)`, l.lockID)
	if err != nil {
		return err
	}

	l.locks.Unregister(l.lockID)

	if l.afterBreak != nil {
		return l.afterBreak()
	}

	return nil
}

func (l *lease) AfterBreak(afterBreakFunc func() error) {
	l.afterBreak = afterBreakFunc
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

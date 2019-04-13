package pool

import (
	"context"
	"time"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgx"
	"github.com/jackc/puddle"
)

type Pool struct {
	p *puddle.Pool
}

// Connect creates a new Pool and immediately establishes one connection. ctx can be used to cancel this initial
// connection.
func Connect(ctx context.Context, connString string) (*Pool, error) {
	p := &Pool{}

	maxConnections := 5 // TODO - unhard-code
	p.p = puddle.NewPool(
		func(ctx context.Context) (interface{}, error) { return pgx.Connect(ctx, connString) },
		func(value interface{}) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			value.(*pgx.Conn).Close(ctx)
			cancel()
		},
		maxConnections)

	// Initially establish one connection
	res, err := p.p.Acquire(ctx)
	if err != nil {
		p.p.Close()
		return nil, err
	}
	res.Release()

	return p, nil
}

// Close closes all connections in the pool and rejects future Acquire calls. Blocks until all connections are returned
// to pool and closed.
func (p *Pool) Close() {
	p.p.Close()
}

func (p *Pool) Acquire(ctx context.Context) (*Conn, error) {
	res, err := p.p.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	return &Conn{res: res}, nil
}

func (p *Pool) Stat() *Stat {
	return &Stat{s: p.p.Stat()}
}

func (p *Pool) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	c, err := p.Acquire(ctx)
	if err != nil {
		return "", err
	}
	defer c.Release()

	return c.Exec(ctx, sql, arguments...)
}

func (p *Pool) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	c, err := p.Acquire(ctx)
	if err != nil {
		return errRows{err: err}, err
	}

	rows, err := c.Query(ctx, sql, args...)
	if err != nil {
		c.Release()
		return errRows{err: err}, err
	}

	return &poolRows{r: rows, c: c}, nil
}

func (p *Pool) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	c, err := p.Acquire(ctx)
	if err != nil {
		return errRow{err: err}
	}

	row := c.QueryRow(ctx, sql, args...)
	return &poolRow{r: row, c: c}
}

func (p *Pool) Begin() (*Tx, error) {
	c, err := p.Acquire(context.Background())
	if err != nil {
		return nil, err
	}

	t, err := c.Begin()
	if err != nil {
		return nil, err
	}

	return &Tx{t: t, c: c}, err
}

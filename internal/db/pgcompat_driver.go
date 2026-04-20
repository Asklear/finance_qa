package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"
)

const PGCompatDriverName = "pgcompat"

func init() {
	sql.Register(PGCompatDriverName, &rewriteDriver{base: stdlib.GetDefaultDriver()})
}

type rewriteDriver struct {
	base driver.Driver
}

func (d *rewriteDriver) Open(name string) (driver.Conn, error) {
	c, err := d.base.Open(name)
	if err != nil {
		return nil, err
	}
	return &rewriteConn{Conn: c}, nil
}

type rewriteConn struct {
	driver.Conn
}

func (c *rewriteConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.Conn.Prepare(rewriteSQL(query))
	if err != nil {
		return nil, err
	}
	return &rewriteStmt{Stmt: s}, nil
}

func (c *rewriteConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if pc, ok := c.Conn.(driver.ConnPrepareContext); ok {
		s, err := pc.PrepareContext(ctx, rewriteSQL(query))
		if err != nil {
			return nil, err
		}
		return &rewriteStmt{Stmt: s}, nil
	}
	return c.Prepare(query)
}

func (c *rewriteConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if ec, ok := c.Conn.(driver.ExecerContext); ok {
		return ec.ExecContext(ctx, rewriteSQL(query), args)
	}
	return nil, driver.ErrSkip
}

func (c *rewriteConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if qc, ok := c.Conn.(driver.QueryerContext); ok {
		return qc.QueryContext(ctx, rewriteSQL(query), args)
	}
	return nil, driver.ErrSkip
}

func (c *rewriteConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if bc, ok := c.Conn.(driver.ConnBeginTx); ok {
		return bc.BeginTx(ctx, opts)
	}
	return c.Conn.Begin()
}

func (c *rewriteConn) Ping(ctx context.Context) error {
	if p, ok := c.Conn.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *rewriteConn) CheckNamedValue(nv *driver.NamedValue) error {
	if cv, ok := c.Conn.(driver.NamedValueChecker); ok {
		return cv.CheckNamedValue(nv)
	}
	return nil
}

type rewriteStmt struct {
	driver.Stmt
}

func (s *rewriteStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if se, ok := s.Stmt.(driver.StmtExecContext); ok {
		return se.ExecContext(ctx, args)
	}
	return nil, driver.ErrSkip
}

func (s *rewriteStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if sq, ok := s.Stmt.(driver.StmtQueryContext); ok {
		return sq.QueryContext(ctx, args)
	}
	return nil, driver.ErrSkip
}

var ifNullRe = regexp.MustCompile(`(?i)\bIFNULL\s*\(`)
var insertOrIgnoreRe = regexp.MustCompile(`(?is)^\s*INSERT\s+OR\s+IGNORE\s+INTO\s+`)
var insertOrReplaceRe = regexp.MustCompile(`(?is)^\s*INSERT\s+OR\s+REPLACE\s+INTO\s+`)

func rewriteSQL(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return query
	}
	q = ifNullRe.ReplaceAllString(q, "COALESCE(")
	if insertOrIgnoreRe.MatchString(q) {
		q = insertOrIgnoreRe.ReplaceAllString(q, "INSERT INTO ")
		if !strings.Contains(strings.ToUpper(q), "ON CONFLICT") {
			q = q + " ON CONFLICT DO NOTHING"
		}
	}
	if insertOrReplaceRe.MatchString(q) {
		q = insertOrReplaceRe.ReplaceAllString(q, "INSERT INTO ")
		if !strings.Contains(strings.ToUpper(q), "ON CONFLICT") {
			// generic fallback without conflict target: keep write idempotent and avoid syntax error
			q = q + " ON CONFLICT DO NOTHING"
		}
	}
	q = rebindQuestionToDollar(q)
	return q
}

func rebindQuestionToDollar(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 16)
	argPos := 1
	inSingle := false
	inDouble := false

	for i := 0; i < len(query); i++ {
		ch := query[i]
		switch ch {
		case '\'':
			if !inDouble {
				if inSingle && i+1 < len(query) && query[i+1] == '\'' {
					b.WriteByte(ch)
					b.WriteByte(query[i+1])
					i++
					continue
				}
				inSingle = !inSingle
			}
			b.WriteByte(ch)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			b.WriteByte(ch)
		case '?':
			if !inSingle && !inDouble {
				b.WriteString(fmt.Sprintf("$%d", argPos))
				argPos++
			} else {
				b.WriteByte(ch)
			}
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}
